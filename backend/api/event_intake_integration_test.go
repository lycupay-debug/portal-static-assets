package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// This opt-in integration test verifies PostgreSQL-backed idempotency under
// concurrent duplicate delivery and protects exact JSONB comparison semantics.
func TestPostgresEventStoreConcurrentDuplicateIntake(t *testing.T) {
	urlFile := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_TEST_DATABASE_URL_FILE"))
	if urlFile == "" {
		t.Skip("set PERSONAL_PORTAL_TEST_DATABASE_URL_FILE to enable PostgreSQL event intake integration test")
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
	db.SetMaxOpenConns(16)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

	store := PostgresEventStore{DB: db}
	const workers = 12
	start := make(chan struct{})
	type outcome struct {
		duplicate bool
		err       error
	}
	outcomes := make(chan outcome, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			duplicate, err := store.ReceiveEvent(ctx, EventInput{
				EventID: eventID, EventType: "test.intake.concurrent",
				Payload: []byte(`{"record":{"a":1,"b":2},"n":9007199254740993}`),
			})
			outcomes <- outcome{duplicate: duplicate, err: err}
		}()
	}
	close(start)
	group.Wait()
	close(outcomes)

	created := 0
	duplicates := 0
	for result := range outcomes {
		if result.err != nil {
			t.Fatalf("concurrent event intake failed: %v", result.err)
		}
		if result.duplicate {
			duplicates++
		} else {
			created++
		}
	}
	if created != 1 || duplicates != workers-1 {
		t.Fatalf("concurrent intake results: created=%d duplicates=%d; want 1 and %d", created, duplicates, workers-1)
	}

	// JSON object key order is semantically irrelevant for JSONB idempotency.
	duplicate, err := store.ReceiveEvent(ctx, EventInput{
		EventID: eventID, EventType: "test.intake.concurrent",
		Payload: []byte(`{"n":9007199254740993,"record":{"b":2,"a":1}}`),
	})
	if err != nil || !duplicate {
		t.Fatalf("same JSONB payload with reordered keys should be a duplicate; duplicate=%t err=%v", duplicate, err)
	}

	// Distinct integers above float64's exact-integer range must not collapse
	// into a false duplicate.
	_, err = store.ReceiveEvent(ctx, EventInput{
		EventID: eventID, EventType: "test.intake.concurrent",
		Payload: []byte(`{"record":{"a":1,"b":2},"n":9007199254740992}`),
	})
	if !errors.Is(err, errEventConflict) {
		t.Fatalf("different large integer must conflict, got %v", err)
	}

	_, err = store.ReceiveEvent(ctx, EventInput{
		EventID: eventID, EventType: "test.intake.other",
		Payload: []byte(`{"record":{"a":1,"b":2},"n":9007199254740993}`),
	})
	if !errors.Is(err, errEventConflict) {
		t.Fatalf("different event type must conflict, got %v", err)
	}
}
