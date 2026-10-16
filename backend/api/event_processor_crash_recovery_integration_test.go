package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const crashRecoveryHelperEnv = "PERSONAL_PORTAL_CRASH_RECOVERY_HELPER"

// This integration test kills a real worker subprocess while its handler is running.
func TestEventProcessorRecoversAfterWorkerProcessCrash(t *testing.T) {
	urlFile := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_TEST_DATABASE_URL_FILE"))
	if urlFile == "" {
		t.Skip("set PERSONAL_PORTAL_TEST_DATABASE_URL_FILE to enable PostgreSQL crash recovery integration test")
	}
	eventID, err := newTestEventUUID()
	if err != nil {
		t.Fatal(err)
	}
	marker := t.TempDir() + "/handler-started"
	db := openCrashRecoveryTestDB(t, urlFile)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM portal.event_inbox WHERE event_id = $1::uuid", eventID)
	}()
	_, err = db.ExecContext(ctx, "INSERT INTO portal.event_inbox (event_id, event_type, payload, dispatch_state, dispatch_notified_at) VALUES ($1::uuid, 'test.processor.crash_recovery', '{\"test\":\"process-crash\"}'::jsonb, 'notified', now())", eventID)
	if err != nil {
		t.Fatal(err)
	}

	child := exec.Command(os.Args[0], "-test.run=^TestEventProcessorCrashRecoverySubprocessHelper$")
	child.Env = append(os.Environ(), crashRecoveryHelperEnv+"=1", "PERSONAL_PORTAL_CRASH_RECOVERY_EVENT_ID="+eventID, "PERSONAL_PORTAL_CRASH_RECOVERY_MARKER="+marker)
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	if err := child.Start(); err != nil {
		t.Fatalf("start worker subprocess: %v", err)
	}
	childFinished := false
	defer func() {
		if !childFinished && child.Process != nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	}()

	markerDeadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(markerDeadline) {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(marker); err != nil {
		_ = child.Process.Kill()
		_ = child.Wait()
		childFinished = true
		t.Fatal("worker subprocess did not claim the event before timeout")
	}
	var status string
	var attempts int
	if err := db.QueryRowContext(ctx, "SELECT status, attempts FROM portal.event_inbox WHERE event_id=$1::uuid", eventID).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "processing" || attempts != 1 {
		t.Fatalf("before crash event state=(%s,%d), want (processing,1)", status, attempts)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatalf("kill worker subprocess to simulate crash: %v", err)
	}
	_ = child.Wait()
	childFinished = true

	// Simulate lease expiry without making the test wait for the production 30-second lease.
	_, err = db.ExecContext(ctx, "UPDATE portal.event_inbox SET processing_lease_until = now() - interval '1 second' WHERE event_id = $1::uuid AND status = 'processing'", eventID)
	if err != nil {
		t.Fatal(err)
	}
	handlerCalls := 0
	restarted := EventProcessor{DB: db, WorkerID: "crash-recovery-restarted-worker", Handlers: map[string]EventHandler{
		"test.processor.crash_recovery": func(context.Context, EventInput) (json.RawMessage, error) {
			handlerCalls++
			return json.RawMessage(`{"recovered":true}`), nil
		},
	}}
	if !restarted.processOne(ctx, restarted.supportedTypes()) {
		t.Fatal("restarted worker did not reclaim the expired event")
	}
	assertEventState(t, ctx, db, eventID, "completed", 2)
	if handlerCalls != 1 {
		t.Fatalf("restarted handler calls=%d, want 1", handlerCalls)
	}
}

func TestEventProcessorCrashRecoverySubprocessHelper(t *testing.T) {
	if os.Getenv(crashRecoveryHelperEnv) != "1" {
		return
	}
	urlFile := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_TEST_DATABASE_URL_FILE"))
	eventID := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_CRASH_RECOVERY_EVENT_ID"))
	marker := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_CRASH_RECOVERY_MARKER"))
	if urlFile == "" || eventID == "" || marker == "" {
		t.Fatal("missing crash recovery helper configuration")
	}
	db := openCrashRecoveryTestDB(t, urlFile)
	defer db.Close()
	processor := EventProcessor{DB: db, WorkerID: "crash-recovery-doomed-worker", Handlers: map[string]EventHandler{
		"test.processor.crash_recovery": func(context.Context, EventInput) (json.RawMessage, error) {
			if err := os.WriteFile(marker, []byte("handler started"), 0600); err != nil {
				t.Fatal(err)
			}
			select {}
		},
	}}
	_ = processor.processOne(context.Background(), processor.supportedTypes())
}

func openCrashRecoveryTestDB(t *testing.T, urlFile string) *sql.DB {
	t.Helper()
	urlBytes, err := os.ReadFile(urlFile)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", strings.TrimSpace(string(urlBytes)))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(4)
	return db
}
