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
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

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
