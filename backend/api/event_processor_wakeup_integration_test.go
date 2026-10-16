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

// This opt-in integration test proves a wake signal triggers a database claim
// without waiting for the periodic recovery scan.
func TestEventProcessorWakesImmediatelyFromSignal(t *testing.T) {
	urlFile := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_TEST_DATABASE_URL_FILE"))
	if urlFile == "" {
		t.Skip("set PERSONAL_PORTAL_TEST_DATABASE_URL_FILE to enable PostgreSQL wakeup integration test")
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

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
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

	wake := make(chan struct{}, 1)
	processor := EventProcessor{
		DB: db,
		Handlers: map[string]EventHandler{"test.processor.wakeup": func(context.Context, EventInput) (json.RawMessage, error) {
			return json.RawMessage(`{"woken":true}`), nil
		}},
		Interval: 5 * time.Second,
		Workers:  1,
		WorkerID: "wakeup-integration",
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

	// Allow the worker's initial empty scan to finish and enter its wait.
	time.Sleep(100 * time.Millisecond)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO portal.event_inbox (event_id, event_type, payload, dispatch_state, dispatch_notified_at)
		VALUES ($1::uuid, 'test.processor.wakeup', '{"trigger":"signal"}'::jsonb, 'notified', now())
	`, eventID); err != nil {
		t.Fatal(err)
	}
	wake <- struct{}{}

	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var status string
		err := db.QueryRowContext(ctx, "SELECT status FROM portal.event_inbox WHERE event_id = $1::uuid", eventID).Scan(&status)
		if err != nil {
			t.Fatal(err)
		}
		if status == "completed" {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("event was not processed after wake signal; last status=%q (periodic interval is 5s)", status)
		case <-ctx.Done():
			t.Fatal("context expired before event completed")
		case <-ticker.C:
		}
	}
}
