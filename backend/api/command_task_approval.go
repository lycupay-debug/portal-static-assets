package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

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
