package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"os/user"
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
		// #nosec G204 -- task claim enforces the execution-mode policy; shell-mode commands are audited and time-limited.
		cmd = exec.CommandContext(commandCtx, task.Executable, "-c", strings.Join(task.Arguments, " "))
	} else {
		// #nosec G204 -- executable/arguments are persisted task inputs guarded by the class allowlist/approval policy and a per-task timeout.
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
		u, lookupErr := user.Lookup("personalportal")
		if lookupErr != nil {
			x.finish(task.ID, 125, "", "", "cannot drop privileges in stable mode: "+lookupErr.Error())
			return true
		}
		uid, uidErr := strconv.ParseUint(u.Uid, 10, 32)
		gid, gidErr := strconv.ParseUint(u.Gid, 10, 32)
		if uidErr != nil || gidErr != nil {
			x.finish(task.ID, 125, "", "", "cannot drop privileges in stable mode: invalid service account UID/GID")
			return true
		}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), Groups: []uint32{uint32(gid)}}
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedBuffer{buffer: &stdout, limit: commandTaskMaxOutput}
	cmd.Stderr = &limitedBuffer{buffer: &stderr, limit: commandTaskMaxOutput}
	if err := cmd.Start(); err != nil {
		x.finish(task.ID, 127, stdout.String(), stderr.String(), "start command: "+err.Error())
		return true
	}
	persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	_, _ = x.DB.ExecContext(persistCtx, `UPDATE portal.command_tasks SET process_id=$2,updated_at=now() WHERE task_id=$1::uuid AND status='running' AND worker_id=$3`, task.ID, cmd.Process.Pid, x.WorkerID)
	persistCancel()
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
				qctx, qcancel := context.WithTimeout(commandCtx, time.Second)
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
	checkCtx, checkCancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
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

var _ io.Writer = (*limitedBuffer)(nil)
