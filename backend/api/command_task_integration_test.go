package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// This integration test uses only the dedicated test database configured by
// PERSONAL_PORTAL_TEST_DATABASE_URL_FILE. It never targets the production DB.
func TestCommandTaskExecutorPostgresIntegration(t *testing.T) {
	urlFile := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_TEST_DATABASE_URL_FILE"))
	if urlFile == "" {
		t.Skip("set PERSONAL_PORTAL_TEST_DATABASE_URL_FILE to enable command-task PostgreSQL integration test")
	}
	urlBytes, err := os.ReadFile(urlFile)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", strings.TrimSpace(string(urlBytes)))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	var originalMode string
	if err := db.QueryRowContext(ctx, `SELECT mode FROM portal.system_execution_policy WHERE singleton=TRUE`).Scan(&originalMode); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE portal.system_execution_policy SET mode='stable' WHERE singleton=TRUE`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), `UPDATE portal.system_execution_policy SET mode=$1 WHERE singleton=TRUE`, originalMode)
	}()

	ids := []string{randomUUID(), randomUUID(), randomUUID(), randomUUID(), randomUUID()}
	defer func() {
		for _, id := range ids {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM portal.command_tasks WHERE task_id=$1::uuid`, id)
		}
		for _, id := range ids {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM portal.compliance_audit_log WHERE object_type='command_task' AND object_id=$1::uuid`, id)
		}
	}()

	insert := func(id, title string, class systemTaskClass, executable string, args []string, approval bool) {
		t.Helper()
		payload, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.ExecContext(ctx, `INSERT INTO portal.command_tasks(task_id,idempotency_key,title,task_class,executable,arguments,working_directory,timeout_seconds,requested_by,mode_at_submission,status,approval_required) VALUES($1::uuid,$2,$3,$4,$5,$6::jsonb,'/var/tmp',15,'integration-test',(SELECT mode FROM portal.system_execution_policy WHERE singleton=TRUE),'queued',$7)`, id, "integration-"+id, title, string(class), executable, string(payload), approval)
		if err != nil {
			t.Fatal(err)
		}
	}
	worker := privilegedTaskExecutor{DB: db, WorkerID: "integration-command-executor", PollInterval: time.Millisecond}

	insert(ids[0], "read-only uname integration", taskReadOnly, "/usr/bin/uname", []string{"-a"}, false)
	if !worker.processOne(ctx) {
		t.Fatal("executor did not claim the queued read-only task")
	}
	var status, stdout string
	var exitCode sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT status,stdout,exit_code FROM portal.command_tasks WHERE task_id=$1::uuid`, ids[0]).Scan(&status, &stdout, &exitCode); err != nil {
		t.Fatal(err)
	}
	if status != "completed" || !exitCode.Valid || exitCode.Int64 != 0 || !strings.Contains(stdout, "FreeBSD") {
		t.Fatalf("read-only task result status=%q exit=%v stdout=%q", status, exitCode, stdout)
	}

	insert(ids[1], "restart approval integration", taskRestart, "/usr/sbin/service", []string{"personal_portal_api", "restart"}, true)
	worker.processOne(ctx) // a gated task is intentionally not returned as executable work
	if err := db.QueryRowContext(ctx, `SELECT status FROM portal.command_tasks WHERE task_id=$1::uuid`, ids[1]).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending_approval" {
		t.Fatalf("unapproved restart status=%q, want pending_approval", status)
	}

	insert(ids[2], "SSH MCP manual-only integration", taskSSHMCP, "/usr/sbin/service", []string{"host-mcp", "restart"}, false)
	worker.processOne(ctx) // SSH MCP tasks are converted to manual-only without execution
	if err := db.QueryRowContext(ctx, `SELECT status FROM portal.command_tasks WHERE task_id=$1::uuid`, ids[2]).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "manual_only" {
		t.Fatalf("SSH MCP task status=%q, want manual_only", status)
	}

	insert(ids[3], "misclassified destructive integration", taskReadOnly, "/bin/rm", []string{"-f", "/var/tmp/never-created-command-task-test-file"}, false)
	worker.processOne(ctx) // direct destructive command classification must stop for approval
	var class string
	if err := db.QueryRowContext(ctx, `SELECT task_class,status FROM portal.command_tasks WHERE task_id=$1::uuid`, ids[3]).Scan(&class, &status); err != nil {
		t.Fatal(err)
	}
	if class != string(taskDestructive) || status != "pending_approval" {
		t.Fatalf("misclassified command was not gated: class=%q status=%q", class, status)
	}

	if _, err := db.ExecContext(ctx, `UPDATE portal.system_execution_policy SET mode='developer' WHERE singleton=TRUE`); err != nil {
		t.Fatal(err)
	}
	insert(ids[4], "developer mode root identity", taskReadOnly, "/usr/bin/id", []string{"-u"}, false)
	if !worker.processOne(ctx) {
		t.Fatal("executor did not claim the developer-mode task")
	}
	if err := db.QueryRowContext(ctx, `SELECT status,stdout,exit_code FROM portal.command_tasks WHERE task_id=$1::uuid`, ids[4]).Scan(&status, &stdout, &exitCode); err != nil {
		t.Fatal(err)
	}
	if status != "completed" || !exitCode.Valid || exitCode.Int64 != 0 || strings.TrimSpace(stdout) != "0" {
		t.Fatalf("developer-mode task did not run as root: status=%q exit=%v stdout=%q", status, exitCode, stdout)
	}
}
