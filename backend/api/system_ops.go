package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type systemLogEntry struct {
	ID      int64          `json:"id"`
	Time    string         `json:"time"`
	Source  string         `json:"source"`
	Level   string         `json:"level"`
	Action  string         `json:"action,omitempty"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type logReadResult struct {
	Items  []systemLogEntry `json:"items"`
	Cursor int64            `json:"cursor"`
}

func (a *portalAPI) requireSystemOpsAdmin(w http.ResponseWriter, r *http.Request) bool {
	u, ok := a.requireUser(w, r)
	if !ok {
		return false
	}
	permission := "portal.system_ops.manage"
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		permission = "portal.system_ops.read"
	}
	var isAdmin bool
	if err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM portal.portal_user_roles ur JOIN portal.role_permissions rp ON rp.role_key=ur.role_key WHERE ur.user_id=$1::uuid AND rp.permission_key=$2)`, u.ID, permission).Scan(&isAdmin); err != nil {
		a.dbError(w, err, "无法验证系统运维权限。")
		return false
	}
	if !isAdmin {
		writeError(w, http.StatusForbidden, "只有管理员可以查看系统运维信息。", nil)
		return false
	}
	return true
}

func systemLogPath(source string) (string, bool) {
	switch source {
	case "server":
		return "/var/log/messages", true
	case "ssh":
		return "/var/log/host-mcp-admin-audit.log", true
	default:
		return "", false
	}
}

func parseSystemLogLine(source string, id int64, line string) systemLogEntry {
	entry := systemLogEntry{ID: id, Source: source, Level: "info", Message: strings.TrimSpace(line)}
	if source == "ssh" {
		var raw struct {
			Time    string         `json:"time"`
			Action  string         `json:"action"`
			Outcome string         `json:"outcome"`
			Details map[string]any `json:"details"`
		}
		if json.Unmarshal([]byte(line), &raw) == nil {
			entry.Time, entry.Action, entry.Details = raw.Time, raw.Action, raw.Details
			entry.Message = raw.Action
			toolSource, _ := raw.Details["tool_source"].(string)
			if toolSource == "Go SSH MCP" {
				operation, _ := raw.Details["operation"].(string)
				entry.Action = goSSHLogAction(operation, raw.Action, raw.Outcome)
				entry.Message = entry.Action
				if raw.Outcome != "" {
					entry.Message += " · " + raw.Outcome
				}
			}
			if raw.Outcome != "" {
				if toolSource != "Go SSH MCP" {
					entry.Message += " · " + raw.Outcome
				}
				switch raw.Outcome {
				case "failed":
					entry.Level = "error"
				case "timeout":
					entry.Level = "warning"
				}
			}
			if raw.Details != nil {
				if rc, ok := raw.Details["returncode"].(float64); ok && rc != 0 {
					entry.Level = "error"
				}
				if timedOut, ok := raw.Details["timed_out"].(bool); ok && timedOut {
					entry.Level = "warning"
				}
				if stage, ok := raw.Details["stage"].(string); ok && stage == "started" {
					entry.Level = "info"
				}
			}
		}
	} else {
		entry.Time = parseServerLogTime(line)
		lower := strings.ToLower(line)
		if strings.Contains(lower, "panic") || strings.Contains(lower, "critical") || strings.Contains(lower, "fatal") || strings.Contains(lower, " error") {
			entry.Level = "error"
		} else if strings.Contains(lower, "warning") || strings.Contains(lower, " warn") || strings.Contains(lower, "failed") || strings.Contains(lower, "failure") {
			entry.Level = "warning"
		}
	}
	return entry
}

func goSSHLogAction(operation, eventType, outcome string) string {
	labels := map[string]string{
		"shell.exec":       "Shell 命令",
		"file.read":        "读取文件",
		"file.write":       "写入文件",
		"file.replace":     "修改文件",
		"file.move":        "移动路径",
		"file.delete":      "删除路径",
		"directory.list":   "列出目录",
		"system.info":      "读取系统信息",
		"service.status":   "查询服务状态",
		"logs.tail":        "读取服务日志",
		"database.query":   "数据库查询",
		"execute_parallel": "并行执行任务",
	}
	label := labels[operation]
	if label == "" {
		label = "执行工具操作"
	}
	var title string
	switch {
	case strings.Contains(eventType, "started"):
		title = "执行 " + label
	case outcome == "timeout":
		title = label + "执行超时"
	case outcome == "failed" || outcome == "blocked" || outcome == "unknown":
		title = label + "执行失败"
	case outcome == "success" && operation == "shell.exec":
		title = "Shell 命令执行结果"
	case outcome == "success":
		title = label
	case eventType == "task.queued" || eventType == "batch.accepted":
		title = "已接收任务 · " + label
	default:
		title = label
	}
	return "Go SSH MCP · " + title
}

// parseServerLogTime returns ISO timestamps so browsers do not guess the year
// (which makes syslog entries appear as 2001) or apply the viewer's timezone.
// PostgreSQL embeds an explicit UTC timestamp in its messages; prefer it over
// the syslog prefix. Other syslog records use the host's configured local zone.
func parseServerLogTime(line string) string {
	if utcAt := strings.Index(line, " UTC"); utcAt > 0 {
		prefix := line[:utcAt]
		for i := 0; i+19 <= len(prefix); i++ {
			if i+10 >= len(prefix) || prefix[i+4] != '-' || prefix[i+7] != '-' {
				continue
			}
			stamp := strings.TrimSpace(prefix[i:])
			for _, layout := range []string{"2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
				if parsed, err := time.Parse(layout, stamp); err == nil {
					return parsed.UTC().In(time.Local).Format(time.RFC3339)
				}
			}
		}
	}
	if len(line) >= 15 {
		if parsed, err := time.ParseInLocation("Jan _2 15:04:05", line[:15], time.Local); err == nil {
			now := time.Now()
			parsed = time.Date(now.Year(), parsed.Month(), parsed.Day(), parsed.Hour(), parsed.Minute(), parsed.Second(), 0, time.Local)
			if parsed.After(now.Add(24 * time.Hour)) {
				parsed = parsed.AddDate(-1, 0, 0)
			}
			return parsed.Format(time.RFC3339)
		}
	}
	return ""
}

// readSystemLog reads only a bounded tail for a new viewer and only appended bytes for a live cursor.
func readSystemLog(path, source string, after int64, limit int) (logReadResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return logReadResult{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return logReadResult{}, err
	}
	if limit < 1 || limit > 200 {
		limit = 100
	}
	size := st.Size()
	initial := after <= 0 || after > size
	if initial {
		start := size - 256*1024
		if start < 0 {
			start = 0
		}
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			return logReadResult{}, err
		}
		cursor := start
		reader := bufio.NewReaderSize(f, 32*1024)
		if start > 0 {
			partial, e := reader.ReadString('\n')
			cursor += int64(len(partial))
			if e != nil && e != io.EOF {
				return logReadResult{}, e
			}
		}
		data, err := io.ReadAll(io.LimitReader(reader, 256*1024))
		if err != nil {
			return logReadResult{}, err
		}
		items := make([]systemLogEntry, 0, limit)
		for len(data) > 0 {
			newline := strings.IndexByte(string(data), '\n')
			if newline < 0 {
				break
			} // Ignore an unfinished final line until its newline arrives.
			line := string(data[:newline])
			data = data[newline+1:]
			cursor += int64(newline + 1)
			items = append(items, parseSystemLogLine(source, cursor, line))
			if len(items) > limit {
				items = items[1:]
			}
		}
		return logReadResult{Items: items, Cursor: cursor}, nil
	}
	if _, err := f.Seek(after, io.SeekStart); err != nil {
		return logReadResult{}, err
	}
	reader := bufio.NewReaderSize(f, 32*1024)
	cursor := after
	items := make([]systemLogEntry, 0, limit)
	for len(items) < limit {
		line, e := reader.ReadString('\n')
		if e != nil && e != io.EOF {
			return logReadResult{}, e
		}
		if e == io.EOF {
			break
		}
		cursor += int64(len(line))
		items = append(items, parseSystemLogLine(source, cursor, strings.TrimSuffix(line, "\n")))
	}
	return logReadResult{Items: items, Cursor: cursor}, nil
}

func systemWarningIsRecent(entry systemLogEntry, now time.Time) bool {
	if entry.Time == "" {
		return false
	}
	var t time.Time
	var err error
	switch {
	case strings.Contains(entry.Time, "T") && strings.HasSuffix(entry.Time, "Z"):
		t, err = time.Parse(time.RFC3339, entry.Time)
	case strings.Contains(entry.Time, "T") && len(entry.Time) >= 5 && (entry.Time[len(entry.Time)-5] == '+' || entry.Time[len(entry.Time)-5] == '-') && !strings.Contains(entry.Time[len(entry.Time)-5:], ":"):
		t, err = time.Parse("2006-01-02T15:04:05-0700", entry.Time)
	case strings.Contains(entry.Time, "T"):
		t, err = time.Parse(time.RFC3339, entry.Time)
	default:
		t, err = time.ParseInLocation("Jan _2 15:04:05", entry.Time, time.Local)
		if err == nil {
			t = t.AddDate(now.Year()-t.Year(), 0, 0)
			if t.After(now.Add(5 * time.Minute)) {
				t = t.AddDate(-1, 0, 0)
			}
		}
	}
	if err != nil {
		return false
	}
	age := now.Sub(t)
	return age >= -5*time.Minute && age <= time.Hour
}

// actionableSystemWarning filters historical noise and test/diagnostic failures out of
// the warning center. Full source logs remain available on the logs page for audit.
func actionableSystemWarning(entry systemLogEntry, now time.Time) bool {
	if entry.Level != "error" && entry.Level != "warning" {
		return false
	}
	if !systemWarningIsRecent(entry, now) {
		return false
	}
	if entry.Source == "ssh" {
		// Read-only shell queries and blocked reads are useful audit entries, but are
		// not service-health incidents. Only failed state-changing operations alert.
		switch entry.Action {
		case "write_file", "replace_in_file", "move_path", "delete_path":
			return entry.Level == "error"
		default:
			return false
		}
	}
	if entry.Source != "server" {
		return false
	}
	lower := strings.ToLower(entry.Message)
	// These are retired schema references, test fixtures, diagnostic probes, or
	// optional host services unrelated to portal availability. Keep them in logs.
	if strings.Contains(lower, "uploaded_image_assets") && strings.Contains(lower, "does not exist") {
		return false
	}
	for _, marker := range []string{
		"permission denied for function pg_ls_dir",
		"go api integration",
		"test.processor.",
		"mit test license text",
		"rc.conf(5)",
	} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	return true
}

type systemExecutionMode string

const (
	systemModeDeveloper systemExecutionMode = "developer"
	systemModeStable    systemExecutionMode = "stable"
)

type systemTaskClass string

const (
	taskReadOnly       systemTaskClass = "read_only"
	taskFileInspection systemTaskClass = "file_inspection"
	taskTest           systemTaskClass = "test"
	taskBuild          systemTaskClass = "build"
	taskCodeChange     systemTaskClass = "code_change"
	taskSystemAdmin    systemTaskClass = "system_admin"
	taskDeploy         systemTaskClass = "deploy"
	taskRestart        systemTaskClass = "restart"
	taskDatabaseChange systemTaskClass = "database_change"
	taskDestructive    systemTaskClass = "destructive"
	taskSSHMCP         systemTaskClass = "ssh_mcp"
)

type taskPolicyDecision struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// decideSystemTaskPolicy is the shared policy contract for task executors.
// Restarts always require explicit approval; SSH_MCP work is manual-only and never run by this worker.
func decideSystemTaskPolicy(mode systemExecutionMode, class systemTaskClass) taskPolicyDecision {
	if class == taskSSHMCP {
		return taskPolicyDecision{Decision: "manual_only", Reason: "SSH_MCP 相关任务只能由用户本人执行，不由任务执行器代执行。"}
	}
	if class == taskRestart {
		return taskPolicyDecision{Decision: "require_approval", Reason: "重启操作必须事先获得用户同意。"}
	}
	if mode == systemModeDeveloper {
		return taskPolicyDecision{Decision: "allow_authorized_scope", Reason: "开发者模式：在本次明确授权的任务范围内自主执行。"}
	}
	switch class {
	case taskReadOnly:
		return taskPolicyDecision{Decision: "allow_read_only", Reason: "稳定运行模式允许只读诊断。"}
	case taskFileInspection:
		return taskPolicyDecision{Decision: "require_path_scope", Reason: "文件检查必须限制在授权路径范围内。"}
	case taskTest:
		return taskPolicyDecision{Decision: "require_registered_task", Reason: "测试必须登记任务并限制执行范围。"}
	case taskBuild:
		return taskPolicyDecision{Decision: "require_workspace_scope", Reason: "构建只能在指定工作区执行。"}
	case taskCodeChange:
		return taskPolicyDecision{Decision: "require_change_scope", Reason: "代码修改必须限制文件、目标和变更范围。"}
	case taskSystemAdmin:
		return taskPolicyDecision{Decision: "require_operation_scope", Reason: "系统管理操作必须明确授权具体对象和操作。"}
	case taskDeploy, taskDatabaseChange, taskDestructive:
		return taskPolicyDecision{Decision: "require_approval", Reason: "稳定运行模式下部署、数据库变更和破坏性操作逐次审批。"}
	default:
		return taskPolicyDecision{Decision: "deny_unknown", Reason: "未知任务分类默认拒绝执行。"}
	}
}

type systemExecutionPolicyResponse struct {
	Mode             systemExecutionMode `json:"mode"`
	UpdatedAt        time.Time           `json:"updatedAt"`
	ChangedBy        string              `json:"changedBy"`
	Enforcement      string              `json:"enforcement"`
	ExecutorStatus   string              `json:"executorStatus"`
	ExecutorLastSeen *time.Time          `json:"executorLastSeen,omitempty"`
	Rules            map[string]any      `json:"rules"`
}

func (a *portalAPI) attachCommandExecutorStatus(ctx context.Context, result *systemExecutionPolicyResponse) {
	var status string
	var lastSeen sql.NullTime
	err := a.db.QueryRowContext(ctx, `SELECT status,last_seen_at FROM portal.command_executor_heartbeat WHERE singleton=TRUE`).Scan(&status, &lastSeen)
	if err != nil {
		result.ExecutorStatus = "offline"
		result.Enforcement = "executor_disconnected"
		return
	}
	result.ExecutorStatus = status
	if lastSeen.Valid {
		result.ExecutorLastSeen = &lastSeen.Time
	}
	if status == "ready" && lastSeen.Valid && time.Since(lastSeen.Time) < 20*time.Second {
		result.Enforcement = "executor_connected"
	} else {
		result.Enforcement = "executor_disconnected"
	}
}

func systemExecutionRules() map[string]any {
	return map[string]any{
		"alwaysRequireApproval": []string{"restart"},
		"alwaysManualOnly":      []string{"ssh_mcp"},
		"developerMode":         "Known task classes run with root privileges within the explicitly authorized scope, except restart tasks which require explicit approval; SSH_MCP-related tasks are always manual-only and never executed by the task worker.",
		"stableMode": map[string]string{
			"read_only": "allow_read_only", "file_inspection": "require_path_scope", "test": "require_registered_task",
			"build": "require_workspace_scope", "code_change": "require_change_scope", "system_admin": "require_operation_scope",
			"deploy": "require_approval", "restart": "require_approval", "database_change": "require_approval",
			"destructive": "require_approval", "ssh_mcp": "manual_only",
		},
	}
}

func (a *portalAPI) systemExecutionPolicy(w http.ResponseWriter, r *http.Request) {
	// systemOps() applies the administrator gate before routing every /system-ops/* request.
	switch r.Method {
	case http.MethodGet:
		var result systemExecutionPolicyResponse
		err := a.db.QueryRowContext(r.Context(), `SELECT mode, updated_at, changed_by FROM portal.system_execution_policy WHERE singleton=TRUE`).Scan(&result.Mode, &result.UpdatedAt, &result.ChangedBy)
		if err != nil {
			a.dbError(w, err, "无法读取服务器运行策略。")
			return
		}
		a.attachCommandExecutorStatus(r.Context(), &result)
		result.Rules = systemExecutionRules()
		writeJSON(w, http.StatusOK, result)
	case http.MethodPut:
		var body struct {
			Mode systemExecutionMode `json:"mode"`
		}
		if err := decodeJSON(w, r, 4096, &body); err != nil {
			writeError(w, http.StatusBadRequest, "运行模式请求格式不正确。", nil)
			return
		}
		if body.Mode != systemModeDeveloper && body.Mode != systemModeStable {
			writeError(w, http.StatusBadRequest, "运行模式只能是 developer 或 stable。", nil)
			return
		}
		u, sessionErr := a.session(r)
		if sessionErr != nil {
			a.dbError(w, sessionErr, "无法读取当前操作人。")
			return
		}
		if u == nil {
			writeError(w, http.StatusUnauthorized, "登录状态已失效，请重新登录。", nil)
			return
		}
		tx, err := a.db.BeginTx(r.Context(), nil)
		if err != nil {
			a.dbError(w, err, "无法开启运行策略变更事务。")
			return
		}
		defer tx.Rollback()
		var previous systemExecutionPolicyResponse
		if err := tx.QueryRowContext(r.Context(), `SELECT mode, updated_at, changed_by FROM portal.system_execution_policy WHERE singleton=TRUE FOR UPDATE`).Scan(&previous.Mode, &previous.UpdatedAt, &previous.ChangedBy); err != nil {
			a.dbError(w, err, "无法锁定当前运行策略。")
			return
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE portal.system_execution_policy SET mode=$1, changed_by=$2, updated_at=now() WHERE singleton=TRUE`, string(body.Mode), u.Email); err != nil {
			a.dbError(w, err, "无法保存运行策略。")
			return
		}
		beforeJSON, _ := json.Marshal(map[string]any{"mode": previous.Mode, "updatedAt": previous.UpdatedAt, "changedBy": previous.ChangedBy})
		afterJSON, _ := json.Marshal(map[string]any{"mode": body.Mode, "changedBy": u.Email, "requestPath": r.URL.Path})
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO portal.compliance_audit_log(actor,action,object_type,object_id,before_state,after_state) VALUES($1,'change_execution_mode','system_execution_policy',NULL,$2::jsonb,$3::jsonb)`, u.Email, string(beforeJSON), string(afterJSON)); err != nil {
			a.dbError(w, err, "无法写入运行策略审计记录，变更未提交。")
			return
		}
		if err := tx.Commit(); err != nil {
			a.dbError(w, err, "运行策略事务提交失败。")
			return
		}
		var result systemExecutionPolicyResponse
		if err := a.db.QueryRowContext(r.Context(), `SELECT mode, updated_at, changed_by FROM portal.system_execution_policy WHERE singleton=TRUE`).Scan(&result.Mode, &result.UpdatedAt, &result.ChangedBy); err != nil {
			a.dbError(w, err, "策略已提交，但无法读取保存结果。")
			return
		}
		a.attachCommandExecutorStatus(r.Context(), &result)
		result.Rules = systemExecutionRules()
		writeJSON(w, http.StatusOK, result)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeError(w, http.StatusMethodNotAllowed, "该运行策略接口只支持 GET 和 PUT。", nil)
	}
}

func (a *portalAPI) systemOps(w http.ResponseWriter, r *http.Request) {
	if !a.requireSystemOpsAdmin(w, r) {
		return
	}
	switch {
	case r.URL.Path == "/system-ops/policy":
		a.systemExecutionPolicy(w, r)
	case r.URL.Path == "/system-ops/tasks":
		switch r.Method {
		case http.MethodGet:
			a.listCommandTasks(w, r)
		case http.MethodPost:
			a.createCommandTask(w, r)
		default:
			w.Header().Set("Allow", "GET, POST")
			writeError(w, http.StatusMethodNotAllowed, "任务队列只支持 GET 和 POST。", nil)
		}
	case strings.HasPrefix(r.URL.Path, "/system-ops/tasks/") && strings.HasSuffix(r.URL.Path, "/approve") && r.Method == http.MethodPost:
		a.approveCommandTask(w, r)
	case strings.HasPrefix(r.URL.Path, "/system-ops/tasks/") && strings.HasSuffix(r.URL.Path, "/cancel") && r.Method == http.MethodPost:
		a.cancelCommandTask(w, r)
	case r.URL.Path == "/system-ops/logs" && r.Method == http.MethodGet:
		source := r.URL.Query().Get("source")
		path, ok := systemLogPath(source)
		if !ok {
			writeError(w, http.StatusBadRequest, "日志来源不正确。", nil)
			return
		}
		after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit == 0 {
			limit = 100
		}
		result, err := readSystemLog(path, source, after, limit)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "当前日志源暂不可读取。", nil)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case r.URL.Path == "/system-ops/warnings" && r.Method == http.MethodGet:
		now := time.Now()
		items := make([]systemLogEntry, 0, 40)
		serverPath, _ := systemLogPath("server")
		server, err := readSystemLog(serverPath, "server", 0, 200)
		if err == nil {
			seen := make(map[string]struct{})
			for i := len(server.Items) - 1; i >= 0 && len(items) < 40; i-- {
				entry := server.Items[i]
				if !actionableSystemWarning(entry, now) {
					continue
				}
				key := strings.ToLower(strings.TrimSpace(entry.Message))
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				items = append(items, entry)
			}
		}
		sshPath, _ := systemLogPath("ssh")
		ssh, err := readSystemLog(sshPath, "ssh", 0, 100)
		if err == nil {
			for i := len(ssh.Items) - 1; i >= 0 && len(items) < 50; i-- {
				if actionableSystemWarning(ssh.Items[i], now) {
					items = append(items, ssh.Items[i])
				}
			}
		}
		var failedCleanup int
		if err := a.db.QueryRowContext(r.Context(), `SELECT count(*)::int FROM portal.event_inbox WHERE event_type='portal.file.cleanup' AND status='failed'`).Scan(&failedCleanup); err != nil && !errors.Is(err, sql.ErrNoRows) {
			a.dbError(w, err, "无法读取系统事件状态。")
			return
		}
		// Surface every failed event family, not just file cleanup. Group by type so
		// one broken event does not flood the warning list with duplicate rows.
		rows, err := a.db.QueryContext(r.Context(), `SELECT event_type, count(*)::int FROM portal.event_inbox WHERE status='failed' GROUP BY event_type ORDER BY count(*) DESC, event_type LIMIT 20`)
		if err != nil {
			a.dbError(w, err, "无法读取系统事件状态。")
			return
		}
		for rows.Next() {
			var eventType string
			var count int
			if err := rows.Scan(&eventType, &count); err != nil {
				rows.Close()
				a.dbError(w, err, "无法读取系统事件状态。")
				return
			}
			items = append(items, systemLogEntry{Source: "server", Level: "error", Time: now.Format(time.RFC3339), Action: "event_dead_lettered", Message: fmt.Sprintf("事件类型 %s 有 %d 个处理失败的事件，需要核验。", eventType, count), Details: map[string]any{"eventType": eventType, "count": count}})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			a.dbError(w, err, "无法读取系统事件状态。")
			return
		}
		rows.Close()
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "failedFileCleanupEvents": failedCleanup, "checkedAt": now.Format(time.RFC3339)})
	case r.URL.Path == "/system-ops/stream" && r.Method == http.MethodGet:
		a.streamSystemLog(w, r)
	default:
		writeError(w, http.StatusNotFound, "未找到系统运维接口。", nil)
	}
}

func (a *portalAPI) streamSystemLog(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	path, ok := systemLogPath(source)
	if !ok {
		writeError(w, http.StatusBadRequest, "日志来源不正确。", nil)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "当前服务不支持实时日志流。", nil)
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if lastID := r.Header.Get("Last-Event-ID"); lastID != "" {
		if parsed, err := strconv.ParseInt(lastID, 10, 64); err == nil {
			after = parsed
		}
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "retry: 2000\n\n")
	flusher.Flush()
	if after <= 0 {
		initial, err := readSystemLog(path, source, 0, 100)
		if err == nil {
			for _, item := range initial.Items {
				if err := writeSSE(w, item); err != nil {
					return
				}
			}
			after = initial.Cursor
			flusher.Flush()
		}
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			result, err := readSystemLog(path, source, after, 100)
			if err != nil {
				continue
			}
			for _, item := range result.Items {
				if err := writeSSE(w, item); err != nil {
					return
				}
				after = item.ID
			}
			if result.Cursor > after || len(result.Items) == 0 {
				after = result.Cursor
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeSSE(w io.Writer, item systemLogEntry) error {
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(w, "id: %d\nevent: log\ndata: %s\n\n", item.ID, data); err != nil {
		return err
	}
	return nil
}
