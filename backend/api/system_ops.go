package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

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
