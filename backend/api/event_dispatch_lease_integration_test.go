package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// A dispatcher whose lease expired must not overwrite state owned by a later
// claimant. dispatch_attempts fences stale completion and retry updates.
func TestEventDispatcherRejectsStaleLeaseUpdates(t *testing.T) {
	urlFile := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_TEST_DATABASE_URL_FILE"))
	if urlFile == "" {
		t.Skip("set PERSONAL_PORTAL_TEST_DATABASE_URL_FILE to enable PostgreSQL dispatch lease integration test")
	}
	db := openCrashRecoveryTestDB(t, urlFile)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
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

	_, err = db.ExecContext(ctx, `
		INSERT INTO portal.event_inbox
			(event_id, event_type, payload, dispatch_state, dispatch_attempts, dispatch_lease_until)
		VALUES ($1::uuid, 'test.dispatch.fencing', '{"case":"stale-lease"}'::jsonb, 'leased', 2, now() + interval '1 minute')
	`, eventID)
	if err != nil {
		t.Fatal(err)
	}

	dispatcher := EventDispatcher{DB: db}
	stale := pendingNotification{EventID: eventID, EventType: "test.dispatch.fencing", Attempts: 1}
	if err := dispatcher.markNotified(ctx, stale); err == nil {
		t.Fatal("stale notification acknowledgement unexpectedly updated a newer lease")
	}
	if err := dispatcher.markRetry(ctx, stale, errors.New("late failure from stale dispatcher")); err == nil {
		t.Fatal("stale retry unexpectedly updated a newer lease")
	}

	var state string
	var attempts int
	if err := db.QueryRowContext(ctx, `
		SELECT dispatch_state, dispatch_attempts
		FROM portal.event_inbox WHERE event_id = $1::uuid
	`, eventID).Scan(&state, &attempts); err != nil {
		t.Fatal(err)
	}
	if state != "leased" || attempts != 2 {
		t.Fatalf("stale updates changed newer lease: state=%q attempts=%d, want leased/2", state, attempts)
	}

	current := pendingNotification{EventID: eventID, EventType: "test.dispatch.fencing", Attempts: 2}
	if err := dispatcher.markNotified(ctx, current); err != nil {
		t.Fatalf("current lease should be allowed to mark notified: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
		SELECT dispatch_state, dispatch_attempts
		FROM portal.event_inbox WHERE event_id = $1::uuid
	`, eventID).Scan(&state, &attempts); err != nil {
		t.Fatal(err)
	}
	if state != "notified" || attempts != 2 {
		t.Fatalf("current lease result: state=%q attempts=%d, want notified/2", state, attempts)
	}
}
