package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

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
