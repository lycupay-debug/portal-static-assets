package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// These tests require a disposable database populated by the numbered
// migrations. They skip in the fast unit-test job and run in the PostgreSQL CI
// job through PORTAL_TEST_DATABASE_URL using the least-privilege app role.
func openPortalIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("PORTAL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PORTAL_TEST_DATABASE_URL is not set; PostgreSQL integration tests are not enabled")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(8)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatalf("ping integration database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newIntegrationEvent(eventType string) EventInput {
	return EventInput{
		EventID:   randomUUID(),
		EventType: eventType,
		Payload:   json.RawMessage(`{"origin":"postgres-integration-test"}`),
	}
}

func cleanupIntegrationEvent(t *testing.T, db *sql.DB, eventID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := db.ExecContext(ctx, "DELETE FROM portal.event_inbox WHERE event_id=$1::uuid", eventID); err != nil {
			t.Errorf("cleanup integration event %s: %v", eventID, err)
		}
	})
}

func insertIntegrationEvent(t *testing.T, db *sql.DB, event EventInput) {
	t.Helper()
	cleanupIntegrationEvent(t, db, event.EventID)
	duplicate, err := (PostgresEventStore{DB: db}).ReceiveEvent(context.Background(), event)
	if err != nil || duplicate {
		t.Fatalf("insert integration event: duplicate=%t err=%v", duplicate, err)
	}
}

func TestPostgresEventStoreConcurrentIdempotency(t *testing.T) {
	db := openPortalIntegrationDB(t)
	event := newIntegrationEvent("portal.test.idempotency")
	cleanupIntegrationEvent(t, db, event.EventID)
	store := PostgresEventStore{DB: db}

	const workers = 16
	start := make(chan struct{})
	var group sync.WaitGroup
	var inserted atomic.Int32
	var duplicates atomic.Int32
	errorsFound := make(chan error, workers)
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			<-start
			duplicate, err := store.ReceiveEvent(context.Background(), event)
			if err != nil {
				errorsFound <- err
				return
			}
			if duplicate {
				duplicates.Add(1)
			} else {
				inserted.Add(1)
			}
		}()
	}
	close(start)
	group.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Errorf("concurrent ReceiveEvent: %v", err)
	}
	if got := inserted.Load(); got != 1 {
		t.Fatalf("new inserts = %d, want exactly 1", got)
	}
	if got := duplicates.Load(); got != workers-1 {
		t.Fatalf("duplicate results = %d, want %d", got, workers-1)
	}

	conflicting := event
	conflicting.Payload = json.RawMessage(`{"origin":"conflicting-replay"}`)
	if duplicate, err := store.ReceiveEvent(context.Background(), conflicting); !errors.Is(err, errEventConflict) || duplicate {
		t.Fatalf("conflicting replay = duplicate %t, error %v; want errEventConflict", duplicate, err)
	}
}

func TestPostgresEventProcessorLeaseOwnershipAndRetry(t *testing.T) {
	db := openPortalIntegrationDB(t)
	event := newIntegrationEvent("portal.test.processing")
	insertIntegrationEvent(t, db, event)

	first := EventProcessor{DB: db, WorkerID: "integration-worker-a"}
	second := EventProcessor{DB: db, WorkerID: "integration-worker-b"}
	types := []string{event.EventType}

	start := make(chan struct{})
	type claimResult struct {
		worker string
		event  *pendingEvent
		err    error
	}
	claimed := make(chan claimResult, 2)
	for _, worker := range []EventProcessor{first, second} {
		go func(processor EventProcessor) {
			<-start
			item, err := processor.claim(context.Background(), types)
			claimed <- claimResult{worker: processor.WorkerID, event: item, err: err}
		}(worker)
	}
	close(start)
	var winner *pendingEvent
	var winnerProcessor EventProcessor
	for range 2 {
		result := <-claimed
		if result.err != nil {
			t.Fatalf("claim event: %v", result.err)
		}
		if result.event != nil {
			if winner != nil {
				t.Fatal("two workers claimed the same single event")
			}
			winner = result.event
			if result.worker == first.WorkerID {
				winnerProcessor = first
			} else {
				winnerProcessor = second
			}
		}
	}
	if winner == nil {
		t.Fatal("no worker claimed the event")
	}
	if winner.Attempts != 1 {
		t.Fatalf("first claim attempts = %d, want 1", winner.Attempts)
	}

	wrongWorker := first
	if winnerProcessor.WorkerID == first.WorkerID {
		wrongWorker = second
	}
	if err := wrongWorker.complete(context.Background(), *winner, json.RawMessage(`{"ok":true}`)); err == nil {
		t.Fatal("non-owner worker completed another worker's lease")
	}
	if err := winnerProcessor.complete(context.Background(), *winner, json.RawMessage(`{"ok":true}`)); err != nil {
		t.Fatalf("lease owner could not complete event: %v", err)
	}

	var status string
	var result string
	if err := db.QueryRow("SELECT status,processing_result::text FROM portal.event_inbox WHERE event_id=$1::uuid", event.EventID).Scan(&status, &result); err != nil {
		t.Fatalf("read completed event: %v", err)
	}
	if status != "completed" || result != `{"ok": true}` {
		t.Fatalf("completed state = %q result=%q", status, result)
	}

	retryEvent := newIntegrationEvent("portal.test.retry")
	insertIntegrationEvent(t, db, retryEvent)
	retryClaim, err := first.claim(context.Background(), []string{retryEvent.EventType})
	if err != nil || retryClaim == nil {
		t.Fatalf("claim retry event: event=%v err=%v", retryClaim, err)
	}
	if err := first.fail(context.Background(), *retryClaim, errors.New("transient test failure")); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}
	var retryStatus string
	var nextProcessing time.Time
	var lastError string
	if err := db.QueryRow("SELECT status,next_processing_at,last_error FROM portal.event_inbox WHERE event_id=$1::uuid", retryEvent.EventID).Scan(&retryStatus, &nextProcessing, &lastError); err != nil {
		t.Fatalf("read retry state: %v", err)
	}
	if retryStatus != "received" || !nextProcessing.After(time.Now()) || lastError != "transient test failure" {
		t.Fatalf("retry state = status:%s next:%s error:%q", retryStatus, nextProcessing, lastError)
	}
}

func TestPostgresEventProcessorExpiresFinalLease(t *testing.T) {
	db := openPortalIntegrationDB(t)
	event := newIntegrationEvent("portal.test.final-lease")
	insertIntegrationEvent(t, db, event)
	_, err := db.Exec("UPDATE portal.event_inbox SET status='processing',attempts=$2,processing_worker_id='crashed-worker',processing_lease_until=now()-interval '1 second' WHERE event_id=$1::uuid", event.EventID, maxProcessingAttempts)
	if err != nil {
		t.Fatalf("prepare expired final lease: %v", err)
	}

	processor := EventProcessor{DB: db, WorkerID: "recovery-worker"}
	claimed, err := processor.claim(context.Background(), []string{event.EventType})
	if err != nil {
		t.Fatalf("recover expired final lease: %v", err)
	}
	if claimed != nil {
		t.Fatalf("final-attempt event should not be reclaimed: %+v", claimed)
	}
	var status string
	var lease sql.NullTime
	if err := db.QueryRow("SELECT status,processing_lease_until FROM portal.event_inbox WHERE event_id=$1::uuid", event.EventID).Scan(&status, &lease); err != nil {
		t.Fatalf("read expired final lease: %v", err)
	}
	if status != "failed" || lease.Valid {
		t.Fatalf("expired final lease state = status:%s lease:%v, want failed with no lease", status, lease)
	}
}

func TestPostgresEventDispatcherRejectsStaleLeaseOwner(t *testing.T) {
	db := openPortalIntegrationDB(t)
	event := newIntegrationEvent("portal.test.dispatch")
	insertIntegrationEvent(t, db, event)

	first := EventDispatcher{DB: db, Batch: 1}
	firstBatch, err := first.claim(context.Background())
	if err != nil || len(firstBatch) != 1 || firstBatch[0].EventID != event.EventID {
		t.Fatalf("first dispatch claim = %#v, err=%v", firstBatch, err)
	}
	_, err = db.Exec("UPDATE portal.event_inbox SET dispatch_lease_until=now()-interval '1 second' WHERE event_id=$1::uuid", event.EventID)
	if err != nil {
		t.Fatalf("expire dispatch lease: %v", err)
	}

	second := EventDispatcher{DB: db, Batch: 1}
	secondBatch, err := second.claim(context.Background())
	if err != nil || len(secondBatch) != 1 || secondBatch[0].EventID != event.EventID {
		t.Fatalf("reclaimed dispatch = %#v, err=%v", secondBatch, err)
	}
	if secondBatch[0].Attempts != firstBatch[0].Attempts+1 {
		t.Fatalf("dispatch attempts = %d, want %d", secondBatch[0].Attempts, firstBatch[0].Attempts+1)
	}
	if err := first.markNotified(context.Background(), firstBatch[0]); err == nil {
		t.Fatal("stale dispatcher marked a reclaimed lease as notified")
	}
	if err := second.markNotified(context.Background(), secondBatch[0]); err != nil {
		t.Fatalf("current dispatcher could not mark notification: %v", err)
	}

	var state string
	if err := db.QueryRow("SELECT dispatch_state FROM portal.event_inbox WHERE event_id=$1::uuid", event.EventID).Scan(&state); err != nil {
		t.Fatalf("read dispatch state: %v", err)
	}
	if state != "notified" {
		t.Fatalf("dispatch state = %q, want notified", state)
	}
}
