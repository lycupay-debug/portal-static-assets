package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Repeated delivery of the same notification must not execute a completed
// durable event more than once.
func TestEventProcessorDuplicateWakeupsDoNotReprocessCompletedEvent(t *testing.T) {
	urlFile := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_TEST_DATABASE_URL_FILE"))
	if urlFile == "" {
		t.Skip("set PERSONAL_PORTAL_TEST_DATABASE_URL_FILE to enable PostgreSQL duplicate-wakeup integration test")
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
	db.SetMaxOpenConns(4)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	eventID, err := newTestEventUUID()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM portal.event_inbox WHERE event_id = $1::uuid", eventID)
	}()

	var handlerCalls atomic.Int32
	wake := make(chan struct{}, 64)
	processor := EventProcessor{
		DB: db,
		Handlers: map[string]EventHandler{"test.processor.duplicate_wakeup": func(context.Context, EventInput) (json.RawMessage, error) {
			handlerCalls.Add(1)
			return json.RawMessage([]byte("{\"processed\":true}")), nil
		}},
		Interval: 5 * time.Second,
		Workers:  1,
		WorkerID: "duplicate-wakeup-integration",
		Wake:     wake,
	}
	processorCtx, stop := context.WithCancel(ctx)
	processorDone := make(chan struct{})
	go func() {
		defer close(processorDone)
		processor.Run(processorCtx)
	}()
	defer func() {
		stop()
		<-processorDone
	}()

	time.Sleep(100 * time.Millisecond)
	if _, err := db.ExecContext(ctx, "INSERT INTO portal.event_inbox (event_id, event_type, payload, dispatch_state, dispatch_notified_at) VALUES ($1::uuid, 'test.processor.duplicate_wakeup', '{\"trigger\":\"duplicate-wakeup\"}'::jsonb, 'notified', now())", eventID); err != nil {
		t.Fatal(err)
	}
	for range 20 {
		wake <- struct{}{}
	}

	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var status string
		if err := db.QueryRowContext(ctx, "SELECT status FROM portal.event_inbox WHERE event_id = $1::uuid", eventID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "completed" {
			break
		}
		select {
		case <-deadline.C:
			t.Fatalf("event did not complete after duplicate wakeups; status=%q", status)
		case <-ctx.Done():
			t.Fatal("context expired before event completed")
		case <-ticker.C:
		}
	}

	for range 20 {
		wake <- struct{}{}
	}
	time.Sleep(150 * time.Millisecond)
	if calls := handlerCalls.Load(); calls != 1 {
		t.Fatalf("handler executed %d times for one completed event, want exactly once", calls)
	}
	var attempts int
	if err := db.QueryRowContext(ctx, "SELECT attempts FROM portal.event_inbox WHERE event_id = $1::uuid", eventID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 {
		t.Fatalf("event attempts=%d after duplicate wakeups, want 1", attempts)
	}
}
