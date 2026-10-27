package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
)

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
