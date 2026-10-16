package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// This destructive integration test is opt-in and deletes only its own fixed
// test event IDs. Point it at a dedicated test database where possible.
func TestEventProcessorPostgresIntegration(t *testing.T) {
	urlFile := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_TEST_DATABASE_URL_FILE"))
	if urlFile == "" {
		t.Skip("set PERSONAL_PORTAL_TEST_DATABASE_URL_FILE to enable PostgreSQL integration test")
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	const successID = "018f2f5e-7a2b-7c4d-8e9f-0123456789ac"
	const retryID = "018f2f5e-7a2b-7c4d-8e9f-0123456789ad"
	const expiredID = "018f2f5e-7a2b-7c4d-8e9f-0123456789ae"
	const terminalID = "018f2f5e-7a2b-7c4d-8e9f-0123456789af"
	const validContentID = "018f2f5e-7a2b-7c4d-8e9f-0123456789b0"
	const invalidContentID = "018f2f5e-7a2b-7c4d-8e9f-0123456789b1"
	ids := []string{successID, retryID, expiredID, terminalID, validContentID, invalidContentID}
	for _, id := range ids {
		if _, err := db.ExecContext(ctx, "DELETE FROM portal.event_inbox WHERE event_id = $1::uuid", id); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		for _, id := range ids {
			_, _ = db.ExecContext(context.Background(), "DELETE FROM portal.event_inbox WHERE event_id = $1::uuid", id)
		}
	}()

	for _, event := range []struct {
		id       string
		kind     string
		status   string
		attempts int
	}{
		{successID, "test.processor.success", "received", 0},
		{retryID, "test.processor.retry", "received", 0},
		{expiredID, "test.processor.recovery", "processing", 1},
		{terminalID, "test.processor.terminal", "received", maxProcessingAttempts - 1},
		{validContentID, contentValidationEventType, "received", 0},
		{invalidContentID, contentValidationEventType, "received", 0},
	} {
		payload := `{"value":7}`
		if event.id == validContentID {
			payload = `{"title":"Valid draft","slug":"valid-draft","content_type":"article","tags":["编程","Python"],"visibility":"public","summary":"Summary","body":"Body"}`
		} else if event.id == invalidContentID {
			payload = `{"title":"","slug":"Bad Slug","content_type":"article","visibility":"private","body":"Body"}`
		}
		_, err := db.ExecContext(ctx, `
			INSERT INTO portal.event_inbox
				(event_id, event_type, payload, status, attempts, processing_lease_until, processing_worker_id)
			VALUES ($1::uuid, $2, $5::jsonb, $3, $4,
			        CASE WHEN $3 = 'processing' THEN now() - interval '1 second' ELSE NULL END,
			        CASE WHEN $3 = 'processing' THEN 'dead-worker' ELSE NULL END)
		`, event.id, event.kind, event.status, event.attempts, payload)
		if err != nil {
			t.Fatal(err)
		}
	}

	processor := EventProcessor{
		DB:       db,
		WorkerID: "integration-worker-success",
		Handlers: map[string]EventHandler{
			"test.processor.success": func(_ context.Context, event EventInput) (json.RawMessage, error) {
				if event.EventID != successID || string(event.Payload) != "{\"value\": 7}" {
					t.Errorf("handler received unexpected event: %#v", event)
				}
				return json.RawMessage(`{"ok":true}`), nil
			},
		},
	}
	if !processor.processOne(ctx, processor.supportedTypes()) {
		t.Fatal("processor did not claim success event")
	}
	assertEventState(t, ctx, db, successID, "completed", 1)
	var storedResult string
	if err := db.QueryRowContext(ctx, "SELECT processing_result::text FROM portal.event_inbox WHERE event_id=$1::uuid", successID).Scan(&storedResult); err != nil {
		t.Fatal(err)
	}
	if storedResult != `{"ok": true}` {
		t.Fatalf("stored processing result = %s, want JSON result", storedResult)
	}

	retryProcessor := EventProcessor{
		DB:       db,
		WorkerID: "integration-worker-retry",
		Handlers: map[string]EventHandler{
			"test.processor.retry": func(context.Context, EventInput) (json.RawMessage, error) {
				return nil, errors.New("expected test failure")
			},
		},
	}
	if !retryProcessor.processOne(ctx, retryProcessor.supportedTypes()) {
		t.Fatal("processor did not claim retry event")
	}
	assertEventState(t, ctx, db, retryID, "received", 1)
	var retryAt time.Time
	if err := db.QueryRowContext(ctx, "SELECT next_processing_at FROM portal.event_inbox WHERE event_id=$1::uuid", retryID).Scan(&retryAt); err != nil {
		t.Fatal(err)
	}
	if !retryAt.After(time.Now()) {
		t.Fatalf("retry scheduled at %s, expected future time", retryAt)
	}

	recoveryProcessor := EventProcessor{
		DB:       db,
		WorkerID: "integration-worker-recovery",
		Handlers: map[string]EventHandler{
			"test.processor.recovery": func(context.Context, EventInput) (json.RawMessage, error) { return nil, nil },
		},
	}
	if !recoveryProcessor.processOne(ctx, recoveryProcessor.supportedTypes()) {
		t.Fatal("processor did not reclaim expired processing lease")
	}
	assertEventState(t, ctx, db, expiredID, "completed", 2)

	terminalProcessor := EventProcessor{
		DB:       db,
		WorkerID: "integration-worker-terminal",
		Handlers: map[string]EventHandler{
			"test.processor.terminal": func(context.Context, EventInput) (json.RawMessage, error) {
				return nil, errors.New("terminal test failure")
			},
		},
	}
	if !terminalProcessor.processOne(ctx, terminalProcessor.supportedTypes()) {
		t.Fatal("processor did not claim terminal failure event")
	}
	assertEventState(t, ctx, db, terminalID, "failed", maxProcessingAttempts)

	contentProcessor := EventProcessor{DB: db, WorkerID: "integration-content-validation", Handlers: registeredEventHandlers()}
	for range 2 {
		if !contentProcessor.processOne(ctx, contentProcessor.supportedTypes()) {
			t.Fatal("content validator did not claim queued validation event")
		}
	}
	assertEventState(t, ctx, db, validContentID, "completed", 1)
	assertEventState(t, ctx, db, invalidContentID, "completed", 1)
	for _, check := range []struct {
		id    string
		valid bool
	}{{validContentID, true}, {invalidContentID, false}} {
		var raw []byte
		if err := db.QueryRowContext(ctx, "SELECT processing_result FROM portal.event_inbox WHERE event_id=$1::uuid", check.id).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var result contentValidationResult
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		if result.Valid != check.valid {
			t.Fatalf("validation result for %s valid=%v, want %v", check.id, result.Valid, check.valid)
		}
	}
}

func assertEventState(t *testing.T, ctx context.Context, db *sql.DB, eventID, wantStatus string, wantAttempts int) {
	t.Helper()
	var status string
	var attempts int
	if err := db.QueryRowContext(ctx,
		"SELECT status, attempts FROM portal.event_inbox WHERE event_id=$1::uuid",
		eventID).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus || attempts != wantAttempts {
		t.Fatalf("event %s state=(%s,%d), want=(%s,%d)", eventID, status, attempts, wantStatus, wantAttempts)
	}
}
