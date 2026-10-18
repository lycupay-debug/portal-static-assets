package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	commandTaskEventType = "portal.command.execute"
	commandTaskMaxOutput = 64 * 1024
	commandTaskLease     = 45 * time.Second
	commandTaskPoll      = 750 * time.Millisecond
)

var commandTaskIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

type commandTaskRequest struct {
	IdempotencyKey string          `json:"idempotencyKey"`
	Title          string          `json:"title"`
	Class          systemTaskClass `json:"taskClass"`
	Executable     string          `json:"executable"`
	Arguments      []string        `json:"arguments"`
	WorkingDir     string          `json:"workingDirectory"`
	ShellMode      bool            `json:"shellMode"`
	TimeoutSeconds int             `json:"timeoutSeconds"`
}

type commandTask struct {
	TaskID           string              `json:"taskId"`
	IdempotencyKey   string              `json:"idempotencyKey"`
	Title            string              `json:"title"`
	Class            systemTaskClass     `json:"taskClass"`
	Executable       string              `json:"executable"`
	Arguments        []string            `json:"arguments"`
	WorkingDir       string              `json:"workingDirectory"`
	ShellMode        bool                `json:"shellMode"`
	TimeoutSeconds   int                 `json:"timeoutSeconds"`
	RequestedBy      string              `json:"requestedBy"`
	ModeAtSubmission systemExecutionMode `json:"modeAtSubmission"`
	Status           string              `json:"status"`
	ApprovalRequired bool                `json:"approvalRequired"`
	ApprovedBy       string              `json:"approvedBy,omitempty"`
	ApprovedAt       *time.Time          `json:"approvedAt,omitempty"`
	CreatedAt        time.Time           `json:"createdAt"`
	UpdatedAt        time.Time           `json:"updatedAt"`
	StartedAt        *time.Time          `json:"startedAt,omitempty"`
	FinishedAt       *time.Time          `json:"finishedAt,omitempty"`
	ExitCode         *int                `json:"exitCode,omitempty"`
	Stdout           string              `json:"stdout"`
	Stderr           string              `json:"stderr"`
	Attempt          int                 `json:"attempt"`
	FailureReason    string              `json:"failureReason,omitempty"`
}

type commandTaskEventPayload struct {
	TaskID string `json:"task_id"`
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func validTaskClass(class systemTaskClass) bool {
	switch class {
	case taskReadOnly, taskFileInspection, taskTest, taskBuild, taskCodeChange, taskSystemAdmin, taskDeploy, taskRestart, taskDatabaseChange, taskDestructive, taskSSHMCP:
		return true
	default:
		return false
	}
}

func allowedTaskWorkingDirectory(path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return false
	}
	for _, root := range []string{"/opt/personal-portal", "/var/tmp", "/var/log/personal-portal"} {
		rel, err := filepath.Rel(root, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

func stableTaskIsSafe(req commandTaskRequest) bool {
	if req.ShellMode || !allowedTaskWorkingDirectory(req.WorkingDir) {
		return false
	}
	base := filepath.Base(req.Executable)
	trustedExecutable := map[string][]string{
		"uname": {"/usr/bin/uname"}, "df": {"/bin/df"}, "freebsd-version": {"/bin/freebsd-version"},
		"uptime": {"/usr/bin/uptime"}, "hostname": {"/bin/hostname"}, "id": {"/usr/bin/id"}, "whoami": {"/usr/bin/whoami"},
		"git": {"/usr/local/bin/git"}, "go127": {"/usr/local/bin/go127"}, "npm": {"/usr/local/bin/npm"}, "make": {"/usr/bin/make"},
		"ls": {"/bin/ls"}, "cat": {"/bin/cat"}, "head": {"/usr/bin/head"}, "tail": {"/usr/bin/tail"},
		"grep": {"/usr/bin/grep"}, "find": {"/usr/bin/find"}, "stat": {"/usr/bin/stat"}, "file": {"/usr/bin/file"},
		"wc": {"/usr/bin/wc"},
	}
	trusted := false
	for _, path := range trustedExecutable[base] {
		if req.Executable == path {
			trusted = true
			break
		}
	}
	if !trusted {
		return false
	}
	for _, arg := range req.Arguments {
		if strings.ContainsRune(arg, 0) || strings.Contains(arg, ".."+string(filepath.Separator)) || strings.Contains(arg, "/etc/") || strings.Contains(arg, "/root/") || strings.Contains(arg, "/var/backups/") || strings.Contains(arg, "/usr/local/etc/") {
			return false
		}
	}
	switch req.Class {
	case taskReadOnly:
		switch base {
		case "uname", "df", "freebsd-version", "uptime", "hostname", "id", "whoami", "pwd", "git", "go127", "npm", "make":
			if base == "git" {
				return len(req.Arguments) > 0 && (req.Arguments[0] == "status" || req.Arguments[0] == "log" || req.Arguments[0] == "diff")
			}
			if base == "go127" {
				return len(req.Arguments) > 0 && req.Arguments[0] == "version"
			}
			if base == "npm" {
				return len(req.Arguments) > 0 && (req.Arguments[0] == "--version" || req.Arguments[0] == "version")
			}
			if base == "make" {
				return len(req.Arguments) > 0 && req.Arguments[0] == "-n"
			}
			return true
		default:
			return false
		}
	case taskFileInspection:
		switch base {
		case "ls", "cat", "head", "tail", "grep", "stat", "file", "wc":
			return true
		case "find":
			for _, arg := range req.Arguments {
				if arg == "-delete" || arg == "-exec" || arg == "-execdir" || arg == "-ok" || arg == "-okdir" || strings.HasPrefix(arg, "-fprint") {
					return false
				}
			}
			return true
		default:
			return false
		}
	case taskTest:
		return (base == "go127" && len(req.Arguments) >= 2 && req.Arguments[0] == "test" && req.Arguments[1] == "./...") ||
			(base == "npm" && len(req.Arguments) == 1 && req.Arguments[0] == "test") ||
			(base == "make" && len(req.Arguments) == 1 && req.Arguments[0] == "test")
	case taskBuild:
		if base == "npm" {
			return len(req.Arguments) == 2 && req.Arguments[0] == "run" && req.Arguments[1] == "build"
		}
		if base == "make" {
			hasTarget, hasAll := false, false
			for _, arg := range req.Arguments {
				if strings.HasPrefix(arg, "TARGET=/var/tmp/personal-portal-task-build-") {
					hasTarget = true
				}
				if arg == "all" {
					hasAll = true
				}
			}
			return hasTarget && hasAll
		}
		if base == "go127" && len(req.Arguments) >= 3 && req.Arguments[0] == "build" {
			for i, arg := range req.Arguments {
				if arg == "-o" && i+1 < len(req.Arguments) && strings.HasPrefix(req.Arguments[i+1], "/var/tmp/personal-portal-task-build-") {
					return true
				}
			}
		}
		return false
	default:
		return false
	}
}

func isSSHMCPCommand(executable string, args []string) bool {
	value := strings.ToLower(executable + " " + strings.Join(args, " "))
	for _, marker := range []string{"ssh_mcp", "ssh-mcp", "host-mcp", "host_mcp", "ssh mcp"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func mandatoryTaskClass(req commandTaskRequest) systemTaskClass {
	if req.Class == taskSSHMCP || isSSHMCPCommand(req.Executable, req.Arguments) {
		return taskSSHMCP
	}
	base := strings.ToLower(filepath.Base(req.Executable))
	if base == "rm" || base == "unlink" || base == "rmdir" || base == "shred" {
		return taskDestructive
	}
	if base == "psql" || base == "createdb" || base == "dropdb" || base == "pg_restore" || base == "pg_dump" || base == "createuser" || base == "dropuser" {
		return taskDatabaseChange
	}
	if base == "chmod" || base == "chown" || base == "sysrc" || base == "zfs" || base == "zpool" || base == "pkg" || base == "mount" || base == "umount" {
		return taskSystemAdmin
	}
	if base == "git" && len(req.Arguments) > 0 {
		switch strings.ToLower(req.Arguments[0]) {
		case "clean", "reset":
			return taskDestructive
		case "apply", "checkout", "switch", "restore", "commit", "push", "merge", "rebase":
			return taskCodeChange
		}
	}
	if base == "npm" && len(req.Arguments) > 0 && (req.Arguments[0] == "install" || req.Arguments[0] == "ci" || req.Arguments[0] == "uninstall" || req.Arguments[0] == "publish") {
		return taskCodeChange
	}
	if base == "make" {
		for _, arg := range req.Arguments {
			if arg == "clean" {
				return taskDestructive
			}
		}
	}
	if base == "service" || base == "rcctl" || base == "systemctl" || base == "launchctl" || base == "shutdown" || base == "reboot" || base == "kill" || base == "pkill" || base == "killall" || strings.HasPrefix(req.Executable, "/usr/local/etc/rc.d/") {
		for _, arg := range req.Arguments {
			lower := strings.ToLower(arg)
			switch lower {
			case "start", "stop", "restart", "reload", "force-reload", "reboot", "shutdown", "-9", "-kill", "-term":
				return taskRestart
			}
			if strings.HasSuffix(lower, "start") || strings.HasSuffix(lower, "stop") || strings.HasSuffix(lower, "restart") || strings.HasSuffix(lower, "reload") {
				return taskRestart
			}
		}
		if base == "shutdown" || base == "reboot" || base == "kill" || base == "pkill" || base == "killall" {
			return taskRestart
		}
	}
	for _, arg := range req.Arguments {
		if strings.Contains(strings.ToLower(arg), "ssh_mcp") || strings.Contains(strings.ToLower(arg), "ssh-mcp") || strings.Contains(strings.ToLower(arg), "host-mcp") || strings.Contains(strings.ToLower(arg), "host_mcp") {
			return taskSSHMCP
		}
	}
	return req.Class
}

func stableApprovedTaskAllowed(req commandTaskRequest) bool {
	if req.ShellMode || !allowedTaskWorkingDirectory(req.WorkingDir) {
		return false
	}
	switch req.Class {
	case taskCodeChange:
		if req.Executable != "/usr/local/bin/git" || req.WorkingDir != "/opt/personal-portal" || len(req.Arguments) < 2 || req.Arguments[0] != "apply" {
			return false
		}
		for _, arg := range req.Arguments {
			if arg == "--unsafe-paths" || strings.Contains(arg, ".."+string(filepath.Separator)) || strings.Contains(arg, "/etc/") || strings.Contains(arg, "/root/") || strings.Contains(arg, "/var/backups/") || strings.Contains(arg, "/usr/local/etc/") {
				return false
			}
		}
		return true
	case taskSystemAdmin:
		switch req.Executable {
		case "/usr/sbin/service", "/usr/sbin/sysrc", "/sbin/zfs", "/sbin/zpool", "/usr/sbin/pkg", "/sbin/mount", "/sbin/umount", "/usr/sbin/chown", "/bin/chmod", "/usr/bin/chmod", "/usr/sbin/pw":
			return true
		}
		return false
	case taskDeploy:
		return req.Executable == "/opt/personal-portal/deploy/freebsd/deploy.sh" && req.WorkingDir == "/opt/personal-portal" && len(req.Arguments) == 0
	case taskRestart:
		return req.Executable == "/usr/sbin/service" && len(req.Arguments) == 2 && (req.Arguments[1] == "restart" || req.Arguments[1] == "start" || req.Arguments[1] == "stop" || req.Arguments[1] == "reload") && !isSSHMCPCommand(req.Executable, req.Arguments)
	case taskDatabaseChange:
		switch req.Executable {
		case "/usr/local/bin/psql", "/usr/local/bin/createdb", "/usr/local/bin/dropdb", "/usr/local/bin/pg_restore", "/usr/local/bin/pg_dump", "/usr/local/bin/createuser", "/usr/local/bin/dropuser":
			return true
		}
		return false
	case taskDestructive:
		switch req.Executable {
		case "/bin/rm", "/bin/rmdir", "/usr/bin/unlink", "/bin/unlink", "/usr/bin/chflags":
			return true
		}
		return false
	default:
		return false
	}
}

func (a *portalAPI) listCommandTasks(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT task_id::text,idempotency_key,title,task_class,executable,arguments::text,working_directory,shell_mode,timeout_seconds,requested_by,mode_at_submission,status,approval_required,COALESCE(approved_by,''),approved_at,created_at,updated_at,started_at,finished_at,exit_code,stdout,stderr,attempt,COALESCE(failure_reason,'') FROM portal.command_tasks ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		a.dbError(w, err, "无法读取任务队列。")
		return
	}
	defer rows.Close()
	items := make([]commandTask, 0)
	for rows.Next() {
		var item commandTask
		var args string
		if err := rows.Scan(&item.TaskID, &item.IdempotencyKey, &item.Title, &item.Class, &item.Executable, &args, &item.WorkingDir, &item.ShellMode, &item.TimeoutSeconds, &item.RequestedBy, &item.ModeAtSubmission, &item.Status, &item.ApprovalRequired, &item.ApprovedBy, &item.ApprovedAt, &item.CreatedAt, &item.UpdatedAt, &item.StartedAt, &item.FinishedAt, &item.ExitCode, &item.Stdout, &item.Stderr, &item.Attempt, &item.FailureReason); err != nil {
			a.dbError(w, err, "读取任务队列时发生错误。")
			return
		}
		if err := json.Unmarshal([]byte(args), &item.Arguments); err != nil {
			a.dbError(w, err, "任务参数格式异常。")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		a.dbError(w, err, "读取任务队列时发生错误。")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (a *portalAPI) createCommandTask(w http.ResponseWriter, r *http.Request) {
	var req commandTaskRequest
	if err := decodeJSON(w, r, 64*1024, &req); err != nil {
		writeError(w, 400, "任务请求格式不正确。", nil)
		return
	}
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.Title = strings.TrimSpace(req.Title)
	if req.Arguments == nil {
		req.Arguments = []string{}
	}
	if len(req.IdempotencyKey) < 8 || len(req.IdempotencyKey) > 160 || len(req.Title) < 3 || len(req.Title) > 160 || !validTaskClass(req.Class) || !filepath.IsAbs(req.Executable) || filepath.Clean(req.Executable) != req.Executable || len(req.Executable) > 512 || len(req.Arguments) > 100 || !allowedTaskWorkingDirectory(req.WorkingDir) {
		writeError(w, 400, "任务字段无效：请检查幂等键、标题、任务分类、绝对执行路径、参数数量和工作目录范围。", nil)
		return
	}
	for _, arg := range req.Arguments {
		if len(arg) > 4096 || strings.ContainsRune(arg, 0) {
			writeError(w, 400, "任务参数超出限制。", nil)
			return
		}
	}
	baseExecutable := strings.ToLower(filepath.Base(req.Executable))
	if req.ShellMode || baseExecutable == "sh" || baseExecutable == "csh" || baseExecutable == "bash" || baseExecutable == "zsh" || baseExecutable == "dash" || baseExecutable == "ksh" || baseExecutable == "fish" {
		writeError(w, 400, "任务队列禁止 Shell 解释执行；请提交明确的可执行文件与参数数组。", nil)
		return
	}
	for _, pair := range []struct{ executable, flag string }{{"python", "-c"}, {"python3", "-c"}, {"python3.14", "-c"}, {"node", "-e"}, {"perl", "-e"}, {"ruby", "-e"}, {"php", "-r"}} {
		if filepath.Base(req.Executable) == pair.executable {
			for _, arg := range req.Arguments {
				if arg == pair.flag {
					writeError(w, 400, "任务队列禁止以内联解释器代码绕过任务分类；请提交受控脚本文件。", nil)
					return
				}
			}
		}
	}
	if mandatory := mandatoryTaskClass(req); mandatory != req.Class {
		req.Class = mandatory
	}
	if req.Class == taskRestart && req.Executable == "/usr/sbin/service" && len(req.Arguments) == 2 && req.Arguments[0] == "personal_portal_task_executor" && req.Arguments[1] == "stop" {
		writeError(w, http.StatusBadRequest, "任务执行器不能通过自己的队列停止自身；请使用主机管理通道执行该操作。", nil)
		return
	}
	if req.TimeoutSeconds == 0 {
		req.TimeoutSeconds = 120
	}
	if req.TimeoutSeconds < 1 || req.TimeoutSeconds > 1800 {
		writeError(w, 400, "任务超时必须在 1 到 1800 秒之间。", nil)
		return
	}
	u, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	var mode systemExecutionMode
	if err := a.db.QueryRowContext(r.Context(), `SELECT mode FROM portal.system_execution_policy WHERE singleton=TRUE`).Scan(&mode); err != nil {
		a.dbError(w, err, "无法读取当前运行模式。")
		return
	}
	decision := decideSystemTaskPolicy(mode, req.Class)
	requiresApproval := decision.Decision == "require_approval"
	if mode == systemModeStable {
		if !stableTaskIsSafe(req) {
			requiresApproval = true
		}
		if req.Class == taskDeploy || req.Class == taskRestart || req.Class == taskDatabaseChange || req.Class == taskDestructive || req.Class == taskSSHMCP || req.Class == taskCodeChange || req.Class == taskSystemAdmin {
			requiresApproval = true
		}
	}
	if req.Class == taskSSHMCP {
		requiresApproval = false
	}
	status := "queued"
	if requiresApproval {
		status = "pending_approval"
	}
	if req.Class == taskSSHMCP {
		status = "manual_only"
	}
	id := randomUUID()
	argsJSON, _ := json.Marshal(req.Arguments)
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dbError(w, err, "无法开启任务事务。")
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), `INSERT INTO portal.command_tasks(task_id,idempotency_key,title,task_class,executable,arguments,working_directory,shell_mode,timeout_seconds,requested_by,mode_at_submission,status,approval_required,failure_reason) VALUES($1::uuid,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10,$11,$12,$13,$14)`, id, req.IdempotencyKey, req.Title, string(req.Class), req.Executable, string(argsJSON), req.WorkingDir, req.ShellMode, req.TimeoutSeconds, u.Email, string(mode), status, requiresApproval, func() string {
		if req.Class == taskSSHMCP {
			return "SSH_MCP 相关任务只能由用户本人在 SSH_MCP 管理通道中执行；此队列不会执行该任务。"
		}
		return ""
	}())
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
			var existing commandTask
			var existingArgs string
			queryErr := a.db.QueryRowContext(r.Context(), `SELECT task_id::text,idempotency_key,title,task_class,executable,arguments::text,working_directory,shell_mode,timeout_seconds,requested_by,mode_at_submission,status,approval_required,created_at,updated_at FROM portal.command_tasks WHERE idempotency_key=$1`, req.IdempotencyKey).Scan(&existing.TaskID, &existing.IdempotencyKey, &existing.Title, &existing.Class, &existing.Executable, &existingArgs, &existing.WorkingDir, &existing.ShellMode, &existing.TimeoutSeconds, &existing.RequestedBy, &existing.ModeAtSubmission, &existing.Status, &existing.ApprovalRequired, &existing.CreatedAt, &existing.UpdatedAt)
			if queryErr == nil {
				_ = json.Unmarshal([]byte(existingArgs), &existing.Arguments)
				if existing.Title == req.Title && existing.Class == req.Class && existing.Executable == req.Executable && existing.WorkingDir == req.WorkingDir && existing.ShellMode == req.ShellMode && existing.TimeoutSeconds == req.TimeoutSeconds && existing.RequestedBy == u.Email && equalStringSlices(existing.Arguments, req.Arguments) {
					writeJSON(w, http.StatusOK, map[string]any{"taskId": existing.TaskID, "status": existing.Status, "approvalRequired": existing.ApprovalRequired, "mode": existing.ModeAtSubmission, "idempotentReplay": true})
					return
				}
			}
			writeError(w, 409, "幂等键已被不同任务内容使用，请使用新的幂等键。", nil)
			return
		}
		a.dbError(w, err, "无法创建任务。")
		return
	}
	before, _ := json.Marshal(map[string]any{"status": "not_created"})
	after, _ := json.Marshal(map[string]any{"taskId": id, "title": req.Title, "taskClass": req.Class, "mode": mode, "status": status, "approvalRequired": requiresApproval, "policyDecision": decision.Decision})
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal.compliance_audit_log(actor,action,object_type,object_id,before_state,after_state) VALUES($1,'create_command_task','command_task',$2::uuid,$3::jsonb,$4::jsonb)`, u.Email, id, string(before), string(after)); err != nil {
		a.dbError(w, err, "无法写入任务审计记录。")
		return
	}
	if status == "queued" {
		payload, _ := json.Marshal(commandTaskEventPayload{TaskID: id})
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal.event_inbox(event_id,event_type,payload) VALUES($1::uuid,$2,$3::jsonb)`, randomUUID(), commandTaskEventType, string(payload)); err != nil {
			a.dbError(w, err, "无法登记任务事件。")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		a.dbError(w, err, "任务事务提交失败。")
		return
	}
	writeJSON(w, 201, map[string]any{"taskId": id, "status": status, "approvalRequired": requiresApproval, "policyDecision": decision.Decision, "mode": mode})
}

func (a *portalAPI) approveCommandTask(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/system-ops/tasks/")
	id = strings.TrimSuffix(id, "/approve")
	if !commandTaskIDPattern.MatchString(id) {
		writeError(w, 400, "任务 ID 格式不正确。", nil)
		return
	}
	u, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dbError(w, err, "无法开启审批事务。")
		return
	}
	defer tx.Rollback()
	var class systemTaskClass
	var status string
	var executable string
	var argsRaw string
	var cwd string
	var shell bool
	err = tx.QueryRowContext(r.Context(), `SELECT task_class,status,executable,arguments::text,working_directory,shell_mode FROM portal.command_tasks WHERE task_id=$1::uuid FOR UPDATE`, id).Scan(&class, &status, &executable, &argsRaw, &cwd, &shell)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "任务不存在。", nil)
		return
	}
	if err != nil {
		a.dbError(w, err, "无法读取待审批任务。")
		return
	}
	if status != "pending_approval" {
		writeError(w, 409, "任务当前状态不允许审批。", []string{"status=" + status})
		return
	}
	if class == taskSSHMCP {
		writeError(w, 409, "SSH_MCP 相关任务不会由任务执行器运行，请由用户本人在 SSH_MCP 管理通道中执行。", nil)
		return
	}
	var mode systemExecutionMode
	if err := tx.QueryRowContext(r.Context(), `SELECT mode FROM portal.system_execution_policy WHERE singleton=TRUE`).Scan(&mode); err != nil {
		a.dbError(w, err, "无法读取当前运行模式。")
		return
	}
	var args []string
	if err := json.Unmarshal([]byte(argsRaw), &args); err != nil {
		a.dbError(w, err, "任务参数异常。")
		return
	}
	req := commandTaskRequest{Class: class, Executable: executable, Arguments: args, WorkingDir: cwd, ShellMode: shell}
	if mode == systemModeStable && !stableTaskIsSafe(req) && !stableApprovedTaskAllowed(req) {
		writeError(w, 403, "稳定模式下该命令不在审批后可执行的受控清单中。", nil)
		return
	}
	if class == taskRestart && isSSHMCPCommand(executable, args) {
		writeError(w, 409, "SSH_MCP/host-mcp 重启必须由用户本人执行，不允许由队列代执行。", nil)
		return
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE portal.command_tasks SET status='queued',approval_required=TRUE,approved_by=$2,approved_at=now(),updated_at=now(),failure_reason=NULL WHERE task_id=$1::uuid`, id, u.Email)
	if err != nil {
		a.dbError(w, err, "无法记录任务审批。")
		return
	}
	before, _ := json.Marshal(map[string]any{"status": status})
	after, _ := json.Marshal(map[string]any{"status": "queued", "approvedBy": u.Email, "approved": true})
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal.compliance_audit_log(actor,action,object_type,object_id,before_state,after_state) VALUES($1,'approve_command_task','command_task',$2::uuid,$3::jsonb,$4::jsonb)`, u.Email, id, string(before), string(after)); err != nil {
		a.dbError(w, err, "无法记录任务审批审计。")
		return
	}
	payload, _ := json.Marshal(commandTaskEventPayload{TaskID: id})
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal.event_inbox(event_id,event_type,payload) VALUES($1::uuid,$2,$3::jsonb)`, randomUUID(), commandTaskEventType, string(payload)); err != nil {
		a.dbError(w, err, "无法唤醒任务执行流程。")
		return
	}
	if err := tx.Commit(); err != nil {
		a.dbError(w, err, "任务审批提交失败。")
		return
	}
	writeJSON(w, 200, map[string]any{"taskId": id, "status": "queued", "approvedBy": u.Email})
}

func (a *portalAPI) cancelCommandTask(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/system-ops/tasks/"), "/cancel")
	if !commandTaskIDPattern.MatchString(id) {
		writeError(w, 400, "任务 ID 格式不正确。", nil)
		return
	}
	u, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dbError(w, err, "无法开启取消任务事务。")
		return
	}
	defer tx.Rollback()
	var previousStatus string
	if err := tx.QueryRowContext(r.Context(), `SELECT status FROM portal.command_tasks WHERE task_id=$1::uuid FOR UPDATE`, id).Scan(&previousStatus); errors.Is(err, sql.ErrNoRows) {
		writeError(w, 409, "任务不存在或已进入终态。", nil)
		return
	} else if err != nil {
		a.dbError(w, err, "无法读取任务状态。")
		return
	}
	if previousStatus != "queued" && previousStatus != "pending_approval" && previousStatus != "running" {
		writeError(w, 409, "任务不存在或已进入终态。", nil)
		return
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE portal.command_tasks SET cancel_requested=TRUE, status=CASE WHEN status IN ('queued','pending_approval') THEN 'cancelled' ELSE status END, finished_at=CASE WHEN status IN ('queued','pending_approval') THEN now() ELSE finished_at END, updated_at=now() WHERE task_id=$1::uuid`, id)
	if err != nil {
		a.dbError(w, err, "无法取消任务。")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal.compliance_audit_log(actor,action,object_type,object_id,before_state,after_state) VALUES($1,'cancel_command_task','command_task',$2::uuid,jsonb_build_object('status',$3),'{"cancelRequested":true}'::jsonb)`, u.Email, id, previousStatus); err != nil {
		a.dbError(w, err, "无法记录取消任务审计，取消未提交。")
		return
	}
	if err := tx.Commit(); err != nil {
		a.dbError(w, err, "取消任务事务提交失败。")
		return
	}
	writeJSON(w, 200, map[string]any{"taskId": id, "cancelRequested": true})
}

func auditCommandTaskState(db *sql.DB, id, action, before, after, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx, `INSERT INTO portal.compliance_audit_log(actor,action,object_type,object_id,before_state,after_state) VALUES('system-executor',$2,'command_task',$1::uuid,jsonb_build_object('status',$3::text),jsonb_build_object('status',$4::text,'reason',NULLIF($5::text,'')))`, id, action, before, after, reason)
	if err != nil {
		log.Printf("command_task_state_audit_failed task_id=%s action=%s error=%v", id, action, err)
	}
}

func commandTaskWakeHandler(ctx context.Context, event EventInput) (json.RawMessage, error) {
	var payload commandTaskEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil || !commandTaskIDPattern.MatchString(payload.TaskID) {
		return nil, errors.New("invalid command task event payload")
	}
	// This event is only a durable wake-up hint. The privileged executor independently
	// reloads policy, approval, and task state from PostgreSQL before it runs anything.
	return json.RawMessage(`{"wakeAccepted":true}`), nil
}

type privilegedTaskExecutor struct {
	DB           *sql.DB
	WorkerID     string
	PollInterval time.Duration
	Wake         <-chan struct{}
}

type claimedCommandTask struct {
	ID             string
	Class          systemTaskClass
	Executable     string
	Arguments      []string
	WorkingDir     string
	ShellMode      bool
	TimeoutSeconds int
	Attempt        int
	Mode           systemExecutionMode
	Approved       bool
}

func (x privilegedTaskExecutor) Run(ctx context.Context) {
	interval := x.PollInterval
	if interval <= 0 {
		interval = commandTaskPoll
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if x.processOne(ctx) {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-x.Wake:
		case <-ticker.C:
		}
	}
}

func (x privilegedTaskExecutor) claim(ctx context.Context) (*claimedCommandTask, error) {
	tx, err := x.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// A stale running record is not retried automatically: the command may have
	// produced non-idempotent side effects before the worker disappeared.
	_, err = tx.ExecContext(ctx, `WITH expired AS (UPDATE portal.command_tasks SET status='failed',finished_at=now(),lease_until=NULL,worker_id=NULL,failure_reason='executor lease expired; manual reconciliation required',updated_at=now() WHERE status='running' AND lease_until < now() RETURNING task_id) INSERT INTO portal.compliance_audit_log(actor,action,object_type,object_id,before_state,after_state) SELECT 'system-executor','expire_command_task','command_task',task_id,'{"status":"running"}'::jsonb,'{"status":"failed","reason":"executor lease expired; manual reconciliation required"}'::jsonb FROM expired`)
	if err != nil {
		return nil, err
	}
	var t claimedCommandTask
	var argsRaw string
	err = tx.QueryRowContext(ctx, `SELECT task_id::text,task_class,executable,arguments::text,working_directory,shell_mode,timeout_seconds,attempt,(approved_at IS NOT NULL AND approved_by IS NOT NULL) FROM portal.command_tasks WHERE status='queued' AND cancel_requested=FALSE ORDER BY created_at,task_id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&t.ID, &t.Class, &t.Executable, &argsRaw, &t.WorkingDir, &t.ShellMode, &t.TimeoutSeconds, &t.Attempt, &t.Approved)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(argsRaw), &t.Arguments); err != nil {
		return nil, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT mode FROM portal.system_execution_policy WHERE singleton=TRUE`).Scan(&t.Mode); err != nil {
		return nil, err
	}
	requestForClassification := commandTaskRequest{Class: t.Class, Executable: t.Executable, Arguments: t.Arguments, WorkingDir: t.WorkingDir, ShellMode: t.ShellMode}
	storedClass := t.Class
	inferredClass := mandatoryTaskClass(requestForClassification)
	baseExecutable := strings.ToLower(filepath.Base(t.Executable))
	if t.ShellMode || baseExecutable == "sh" || baseExecutable == "csh" || baseExecutable == "bash" || baseExecutable == "zsh" || baseExecutable == "dash" || baseExecutable == "ksh" || baseExecutable == "fish" {
		_, err = tx.ExecContext(ctx, `UPDATE portal.command_tasks SET status='denied',failure_reason='任务执行器拒绝 Shell 解释执行；请使用明确的可执行文件与参数数组',finished_at=now(),updated_at=now() WHERE task_id=$1::uuid`, t.ID)
		if err != nil {
			return nil, err
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		auditCommandTaskState(x.DB, t.ID, "deny_shell_mode", "queued", "denied", "shell execution is disabled")
		return nil, nil
	}
	if inferredClass == taskSSHMCP {
		_, err = tx.ExecContext(ctx, `UPDATE portal.command_tasks SET task_class='ssh_mcp',status='manual_only',approval_required=FALSE,approved_by=NULL,approved_at=NULL,failure_reason='SSH_MCP 相关任务只能由用户本人执行，不由任务执行器代执行。',updated_at=now() WHERE task_id=$1::uuid`, t.ID)
		if err != nil {
			return nil, err
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		auditCommandTaskState(x.DB, t.ID, "mark_manual_only", string(storedClass), "manual_only", "SSH_MCP task requires the user to execute it directly")
		return nil, nil
	}
	if storedClass != inferredClass {
		_, err = tx.ExecContext(ctx, `UPDATE portal.command_tasks SET task_class=$2,status='pending_approval',approval_required=TRUE,approved_by=NULL,approved_at=NULL,failure_reason='执行器检测到任务包含重启或 SSH_MCP 特征，已强制重新分类并要求重新审批',updated_at=now() WHERE task_id=$1::uuid`, t.ID, string(inferredClass))
		if err != nil {
			return nil, err
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		auditCommandTaskState(x.DB, t.ID, "force_reclassify_task", string(storedClass), "pending_approval", "command inspection requires a higher-risk class")
		return nil, nil
	}
	t.Class = inferredClass
	if t.Class == taskRestart && t.Executable == "/usr/sbin/service" && len(t.Arguments) == 2 && t.Arguments[0] == "personal_portal_task_executor" && t.Arguments[1] == "stop" {
		_, err = tx.ExecContext(ctx, `UPDATE portal.command_tasks SET status='denied',failure_reason='任务执行器不能通过自己的队列停止自身。',finished_at=now(),updated_at=now() WHERE task_id=$1::uuid`, t.ID)
		if err != nil {
			return nil, err
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		auditCommandTaskState(x.DB, t.ID, "deny_executor_self_stop", "queued", "denied", "self-stop would prevent task completion")
		return nil, nil
	}
	decision := decideSystemTaskPolicy(t.Mode, t.Class)
	if decision.Decision == "require_approval" {
		var approved bool
		if err = tx.QueryRowContext(ctx, `SELECT approved_at IS NOT NULL AND approved_by IS NOT NULL FROM portal.command_tasks WHERE task_id=$1::uuid`, t.ID).Scan(&approved); err != nil {
			return nil, err
		}
		if !approved {
			_, err = tx.ExecContext(ctx, `UPDATE portal.command_tasks SET status='pending_approval',approval_required=TRUE,updated_at=now() WHERE task_id=$1::uuid`, t.ID)
			if err != nil {
				return nil, err
			}
			if err = tx.Commit(); err != nil {
				return nil, err
			}
			auditCommandTaskState(x.DB, t.ID, "require_approval", "queued", "pending_approval", "restart or other policy rule requires explicit approval")
			return nil, nil
		}
	}
	if t.Mode == systemModeStable && !stableTaskIsSafe(commandTaskRequest{Class: t.Class, Executable: t.Executable, Arguments: t.Arguments, WorkingDir: t.WorkingDir, ShellMode: t.ShellMode}) {
		var approved bool
		if err = tx.QueryRowContext(ctx, `SELECT approved_at IS NOT NULL FROM portal.command_tasks WHERE task_id=$1::uuid`, t.ID).Scan(&approved); err != nil {
			return nil, err
		}
		if !approved {
			_, err = tx.ExecContext(ctx, `UPDATE portal.command_tasks SET status='pending_approval',approval_required=TRUE,updated_at=now(),failure_reason='稳定模式需要额外审批或任务范围不符合自动执行规则' WHERE task_id=$1::uuid`, t.ID)
			if err != nil {
				return nil, err
			}
			if err = tx.Commit(); err != nil {
				return nil, err
			}
			auditCommandTaskState(x.DB, t.ID, "require_stable_scope_approval", "queued", "pending_approval", "stable-mode task is outside the automatic command allowlist")
			return nil, nil
		}
		// Even with approval, stable mode only elevates commands from the explicit per-class allowlist.
		if !stableApprovedTaskAllowed(commandTaskRequest{Class: t.Class, Executable: t.Executable, Arguments: t.Arguments, WorkingDir: t.WorkingDir, ShellMode: t.ShellMode}) {
			_, err = tx.ExecContext(ctx, `UPDATE portal.command_tasks SET status='denied',failure_reason='稳定模式下该命令不在审批后可执行的受控清单中。',finished_at=now(),updated_at=now() WHERE task_id=$1::uuid`, t.ID)
			if err != nil {
				return nil, err
			}
			if err = tx.Commit(); err != nil {
				return nil, err
			}
			auditCommandTaskState(x.DB, t.ID, "deny_stable_command", "queued", "denied", "command is outside the approved stable-mode allowlist")
			return nil, nil
		}
	}
	if t.Class == taskRestart && isSSHMCPCommand(t.Executable, t.Arguments) {
		_, err = tx.ExecContext(ctx, `UPDATE portal.command_tasks SET status='manual_only',failure_reason='SSH_MCP/host-mcp 重启必须由用户本人执行。',updated_at=now() WHERE task_id=$1::uuid`, t.ID)
		if err != nil {
			return nil, err
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	t.Attempt++
	_, err = tx.ExecContext(ctx, `UPDATE portal.command_tasks SET status='running',worker_id=$2,lease_until=now()+interval '45 seconds',started_at=now(),attempt=$3,updated_at=now(),failure_reason=NULL WHERE task_id=$1::uuid`, t.ID, x.WorkerID, t.Attempt)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &t, nil
}

func replaceEnvironmentValue(environment []string, key, value string) []string {
	prefix := key + "="
	filtered := make([]string, 0, len(environment)+1)
	for _, item := range environment {
		if !strings.HasPrefix(item, prefix) {
			filtered = append(filtered, item)
		}
	}
	return append(filtered, prefix+value)
}

func boundedOutput(data []byte) string {
	if len(data) <= commandTaskMaxOutput {
		return string(data)
	}
	return string(data[len(data)-commandTaskMaxOutput:])
}

func (x privilegedTaskExecutor) processOne(ctx context.Context) bool {
	claimCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	task, err := x.claim(claimCtx)
	cancel()
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("command_task_claim_failed error=%v", err)
		}
		return false
	}
	if task == nil {
		return false
	}
	if task.Class == taskRestart && task.Executable == "/usr/sbin/service" && len(task.Arguments) == 2 && task.Arguments[0] == "personal_portal_task_executor" && task.Arguments[1] == "restart" {
		x.finish(task.ID, 0, "Task executor restart accepted; rc.d supervisor will launch the current binary.", "", "")
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
		return true
	}
	commandCtx, stop := context.WithTimeout(ctx, time.Duration(task.TimeoutSeconds)*time.Second)
	defer stop()
	var cmd *exec.Cmd
	if task.ShellMode {
		cmd = exec.CommandContext(commandCtx, task.Executable, "-c", strings.Join(task.Arguments, " "))
	} else {
		cmd = exec.CommandContext(commandCtx, task.Executable, task.Arguments...)
	}
	cmd.Dir = task.WorkingDir
	if task.Mode == systemModeStable {
		cmd.Env = replaceEnvironmentValue(os.Environ(), "HOME", "/var/db/personal-portal/task-home")
		cmd.Env = replaceEnvironmentValue(cmd.Env, "GOCACHE", "/var/db/personal-portal/task-home/go-build")
		cmd.Env = replaceEnvironmentValue(cmd.Env, "GOMODCACHE", "/var/db/personal-portal/task-home/go/pkg/mod")
	}
	if task.Class == taskDeploy {
		env := cmd.Env
		if env == nil {
			env = os.Environ()
		}
		cmd.Env = replaceEnvironmentValue(env, "PERSONAL_PORTAL_RELEASE_MODE", "stable")
		cmd.Env = replaceEnvironmentValue(cmd.Env, "PERSONAL_PORTAL_TASK_EXECUTOR_DEPLOY", "1")
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Stable-mode tasks run with the service account's lower privilege. Developer
	// mode deliberately runs as root, matching the administrator's explicit mode.
	runAsRoot := task.Mode == systemModeDeveloper || (task.Approved && (task.Class == taskDeploy || task.Class == taskRestart || task.Class == taskDatabaseChange || task.Class == taskDestructive || task.Class == taskCodeChange || task.Class == taskSystemAdmin))
	if !runAsRoot {
		if u, err := user.Lookup("personalportal"); err == nil {
			uid, _ := strconv.Atoi(u.Uid)
			gid, _ := strconv.Atoi(u.Gid)
			cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), Groups: []uint32{uint32(gid)}}
		} else {
			x.finish(task.ID, 125, "", "", "cannot drop privileges in stable mode: "+err.Error())
			return true
		}
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedBuffer{buffer: &stdout, limit: commandTaskMaxOutput}
	cmd.Stderr = &limitedBuffer{buffer: &stderr, limit: commandTaskMaxOutput}
	if err := cmd.Start(); err != nil {
		x.finish(task.ID, 127, stdout.String(), stderr.String(), "start command: "+err.Error())
		return true
	}
	_, _ = x.DB.ExecContext(context.Background(), `UPDATE portal.command_tasks SET process_id=$2,updated_at=now() WHERE task_id=$1::uuid AND status='running' AND worker_id=$3`, task.ID, cmd.Process.Pid, x.WorkerID)
	monitorDone := make(chan struct{})
	monitorStop := make(chan struct{})
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-commandCtx.Done():
				return
			case <-monitorStop:
				return
			case <-ticker.C:
				var cancelRequested bool
				qctx, qcancel := context.WithTimeout(context.Background(), time.Second)
				err := x.DB.QueryRowContext(qctx, `UPDATE portal.command_tasks SET lease_until=now()+interval '45 seconds',updated_at=now() WHERE task_id=$1::uuid AND status='running' AND worker_id=$2 RETURNING cancel_requested`, task.ID, x.WorkerID).Scan(&cancelRequested)
				qcancel()
				if err == nil && cancelRequested {
					stop()
					return
				}
			}
		}
	}()
	waitErr := cmd.Wait()
	cancelledOrTimedOut := commandCtx.Err() != nil
	stop()
	close(monitorStop)
	<-monitorDone
	if cancelledOrTimedOut {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		time.Sleep(250 * time.Millisecond)
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	exitCode := 0
	failure := ""
	if waitErr != nil {
		failure = waitErr.Error()
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) {
			exitCode = ee.ExitCode()
		} else {
			exitCode = 127
		}
	}
	if cancelledOrTimedOut {
		exitCode = 124
		failure = "task timed out or executor shutting down"
	}
	if ctx.Err() != nil {
		failure = "executor shutting down; process group terminated"
	}
	var cancelRequested bool
	checkCtx, checkCancel := context.WithTimeout(context.Background(), time.Second)
	_ = x.DB.QueryRowContext(checkCtx, `SELECT cancel_requested FROM portal.command_tasks WHERE task_id=$1::uuid`, task.ID).Scan(&cancelRequested)
	checkCancel()
	if cancelRequested {
		exitCode = 130
		failure = "task cancelled by administrator"
	}
	x.finish(task.ID, exitCode, boundedOutput(stdout.Bytes()), boundedOutput(stderr.Bytes()), failure)
	log.Printf("command_task_finished task_id=%s class=%s exit_code=%d duration_timeout=%t", task.ID, task.Class, exitCode, cancelledOrTimedOut)
	if task.Class == taskDeploy && exitCode == 0 {
		// The deployment replaced this executable on disk. Exit gracefully after
		// committing the task result so rc.d's supervisor starts the new binary.
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	}
	return true
}

type limitedBuffer struct {
	buffer *bytes.Buffer
	limit  int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	combined := append(append([]byte(nil), b.buffer.Bytes()...), p...)
	if len(combined) > b.limit {
		combined = combined[len(combined)-b.limit:]
	}
	b.buffer.Reset()
	_, _ = b.buffer.Write(combined)
	return n, nil
}

func (x privilegedTaskExecutor) finish(id string, exitCode int, stdout, stderr, failure string) {
	if len(stdout) > commandTaskMaxOutput {
		stdout = boundedOutput([]byte(stdout))
	}
	if len(stderr) > commandTaskMaxOutput {
		stderr = boundedOutput([]byte(stderr))
	}
	if len(failure) > 2000 {
		failure = failure[:2000]
	}
	status := "completed"
	if exitCode != 0 {
		status = "failed"
	}
	if exitCode == 130 && strings.Contains(strings.ToLower(failure), "cancelled") {
		status = "cancelled"
	}
	if failure == "" && exitCode != 0 {
		failure = "command exited with non-zero status"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := x.DB.BeginTx(ctx, nil)
	if err != nil {
		log.Printf("command_task_finish_begin_failed task_id=%s error=%v", id, err)
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE portal.command_tasks SET status=$2,exit_code=$3,stdout=$4,stderr=$5,failure_reason=NULLIF($6,''),finished_at=now(),lease_until=NULL,worker_id=NULL,process_id=NULL,updated_at=now() WHERE task_id=$1::uuid AND status='running' AND worker_id=$7`, id, status, exitCode, stdout, stderr, failure, x.WorkerID)
	if err != nil {
		log.Printf("command_task_finish_update_failed task_id=%s error=%v", id, err)
		return
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		if err != nil {
			log.Printf("command_task_finish_rows_failed task_id=%s error=%v", id, err)
		}
		return
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO portal.compliance_audit_log(actor,action,object_type,object_id,before_state,after_state) VALUES('system-executor','finish_command_task','command_task',$1::uuid,'{}'::jsonb,jsonb_build_object('status',$2::text,'exitCode',$3::int,'failureReason',NULLIF($4::text,'')))`, id, status, exitCode, failure); err != nil {
		log.Printf("command_task_finish_audit_failed task_id=%s error=%v", id, err)
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf("command_task_finish_commit_failed task_id=%s error=%v", id, err)
	}
}

func (x privilegedTaskExecutor) CancelRunning(ctx context.Context) {
	rows, err := x.DB.QueryContext(ctx, `SELECT task_id::text,process_id FROM portal.command_tasks WHERE status='running' AND cancel_requested=TRUE AND process_id IS NOT NULL`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var pid int
		if rows.Scan(&id, &pid) == nil && pid > 1 {
			_ = syscall.Kill(-pid, syscall.SIGTERM)
			time.Sleep(200 * time.Millisecond)
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			x.finish(id, 130, "", "", "task cancelled by administrator")
		}
	}
}

func runCommandTaskExecutor(ctx context.Context, db *sql.DB) {
	workerID := fmt.Sprintf("root-executor-%d-%d", os.Getpid(), time.Now().UnixNano())
	socketPath := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_EVENT_SOCKET"))
	if socketPath == "" {
		socketPath = defaultSocket
	}
	wake := make(chan struct{}, 256)
	ipc := IPCClient{SocketPath: socketPath, Timeout: 750 * time.Millisecond}
	go ipc.Subscribe(ctx, wake)
	_, err := db.ExecContext(ctx, `INSERT INTO portal.command_executor_heartbeat(singleton,worker_id,status,started_at,last_seen_at,last_error) VALUES(TRUE,$1,'ready',now(),now(),'') ON CONFLICT (singleton) DO UPDATE SET worker_id=EXCLUDED.worker_id,status='ready',started_at=now(),last_seen_at=now(),last_error=''`, workerID)
	if err != nil {
		log.Printf("command_executor_heartbeat_start_failed error=%v", err)
	}
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				qctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				_, e := db.ExecContext(qctx, `UPDATE portal.command_executor_heartbeat SET status='ready',last_seen_at=now(),last_error='' WHERE singleton=TRUE AND worker_id=$1`, workerID)
				cancel()
				if e != nil {
					log.Printf("command_executor_heartbeat_failed error=%v", e)
				}
			}
		}
	}()
	executor := privilegedTaskExecutor{DB: db, WorkerID: workerID, PollInterval: commandTaskPoll, Wake: wake}
	executor.Run(ctx)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = db.ExecContext(shutdownCtx, `UPDATE portal.command_executor_heartbeat SET status='stopping',last_seen_at=now() WHERE singleton=TRUE AND worker_id=$1`, workerID)
	executor.CancelRunning(shutdownCtx)
	<-heartbeatDone
	_, _ = db.ExecContext(shutdownCtx, `UPDATE portal.command_executor_heartbeat SET status='offline',last_seen_at=now() WHERE singleton=TRUE AND worker_id=$1`, workerID)
}

var _ io.Writer = (*limitedBuffer)(nil)
