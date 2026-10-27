package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
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
