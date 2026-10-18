package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// This opt-in end-to-end test submits through the HTTP handler, processes the
// persisted event with the real registered handler, and verifies the database result.
func TestContentValidationHTTPToPostgresIntegration(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	const eventID = "018f2f5e-7a2b-7c4d-8e9f-0123456789b2"
	if _, err := db.ExecContext(ctx, "DELETE FROM portal.event_inbox WHERE event_id = $1::uuid", eventID); err != nil {
		t.Fatal(err)
	}
	// processOne claims the oldest eligible event of a registered type. The
	// dispatcher also claims the oldest eligible notification. Refuse to run
	// against this shared database if either query could pick unrelated work.
	var otherEligible int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM portal.event_inbox
		WHERE event_type = 'portal.content.validate'
		  AND event_id <> $1::uuid
		  AND ((status = 'received' AND next_processing_at <= now())
		       OR (status = 'processing' AND processing_lease_until < now()))
	`, eventID).Scan(&otherEligible); err != nil {
		t.Fatal(err)
	}
	if otherEligible != 0 {
		t.Fatalf("refusing to run: found %d other eligible content validation events", otherEligible)
	}
	var otherDispatchEligible int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM portal.event_inbox
		WHERE event_id <> $1::uuid
		  AND ((dispatch_state = 'pending' AND next_dispatch_at <= now())
		       OR (dispatch_state = 'leased' AND dispatch_lease_until < now()))
	`, eventID).Scan(&otherDispatchEligible); err != nil {
		t.Fatal(err)
	}
	if otherDispatchEligible != 0 {
		t.Fatalf("refusing to run: found %d other eligible dispatch events", otherDispatchEligible)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM portal.event_inbox WHERE event_id = $1::uuid", eventID)
	}()

	requestBody := []byte(`{
		"event_id":"018f2f5e-7a2b-7c4d-8e9f-0123456789b2",
		"event_type":"portal.content.validate",
		"payload":{
			"title":"HTTP integration draft",
			"slug":"http-integration-draft",
			"content_type":"article",
			"tags":["编程","Python"],
			"visibility":"private",
			"summary":"A test summary",
			"body":"A test body"
		}
	}`)

	handler := receiveEventHandler(PostgresEventStore{DB: db})
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(requestBody)))
	if first.Code != http.StatusAccepted {
		t.Fatalf("first POST status = %d, body=%s; want %d", first.Code, first.Body.String(), http.StatusAccepted)
	}

	duplicate := httptest.NewRecorder()
	handler.ServeHTTP(duplicate, httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(requestBody)))
	if duplicate.Code != http.StatusOK {
		t.Fatalf("duplicate POST status = %d, body=%s; want %d", duplicate.Code, duplicate.Body.String(), http.StatusOK)
	}

	processor := EventProcessor{
		DB:       db,
		WorkerID: "http-integration-content-validator",
		Handlers: map[string]EventHandler{contentValidationEventType: validateContentEvent},
	}
	processed := false
	for range 20 {
		if !processor.processOne(ctx, processor.supportedTypes()) {
			break
		}
		var status string
		if err := db.QueryRowContext(ctx,
			"SELECT status FROM portal.event_inbox WHERE event_id = $1::uuid", eventID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "completed" {
			processed = true
			break
		}
	}
	if !processed {
		var status string
		var lastError sql.NullString
		err := db.QueryRowContext(ctx,
			"SELECT status, last_error FROM portal.event_inbox WHERE event_id = $1::uuid", eventID).
			Scan(&status, &lastError)
		if err != nil {
			t.Fatal(err)
		}
		t.Fatalf("event was not completed; status=%q last_error=%q", status, lastError.String)
	}

	// Dispatch is independent of business status: a fast processor may finish
	// the event before the dispatcher runs, but the notification must remain eligible.
	dispatcher := EventDispatcher{DB: db, Batch: 1}
	notifications, err := dispatcher.claim(ctx)
	if err != nil {
		t.Fatalf("claim completed event for dispatch: %v", err)
	}
	if len(notifications) != 1 {
		t.Fatalf("dispatch claim returned %d events, want 1", len(notifications))
	}
	if notifications[0].EventID != eventID || notifications[0].EventType != "portal.content.validate" {
		t.Fatalf("dispatch claim returned unrelated event: %#v", notifications[0])
	}

	var status string
	var raw []byte
	if err := db.QueryRowContext(ctx,
		"SELECT status, processing_result FROM portal.event_inbox WHERE event_id = $1::uuid", eventID).
		Scan(&status, &raw); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatalf("stored status = %q, want completed", status)
	}
	var result contentValidationResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode processing_result: %v; raw=%s", err, raw)
	}
	if !result.Valid {
		t.Fatalf("content validation result is invalid: %s", raw)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("valid content has validation errors: %#v", result.Errors)
	}
}
