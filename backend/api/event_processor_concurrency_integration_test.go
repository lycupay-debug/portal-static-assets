package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// This opt-in integration test uses unique temporary rows and deletes only those rows.
// Prefer a dedicated test database; production runs should be explicitly authorized.
func TestEventProcessorRunsDistinctEventsConcurrently(t *testing.T) {
	urlFile := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_TEST_DATABASE_URL_FILE"))
	if urlFile == "" {
		t.Skip("set PERSONAL_PORTAL_TEST_DATABASE_URL_FILE to enable PostgreSQL concurrency integration test")
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
	db.SetMaxOpenConns(8)

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	var queued int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM portal.event_inbox WHERE status IN ('received', 'processing')`).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 0 {
		t.Fatalf("refusing concurrency test while %d production events are received or processing", queued)
	}

	const eventCount = 4
	const eventType = "test.processor.concurrent"
	ids := make([]string, 0, eventCount)
	for range eventCount {
		id, err := newTestEventUUID()
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		for _, id := range ids {
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM portal.event_inbox WHERE event_id = $1::uuid", id)
		}
	}()

	for index, id := range ids {
		payload, _ := json.Marshal(map[string]int{"sequence": index})
		_, err := db.ExecContext(ctx, `
			INSERT INTO portal.event_inbox
				(event_id, event_type, payload, dispatch_state, dispatch_notified_at)
			VALUES ($1::uuid, $2, $3::jsonb, 'notified', now())
		`, id, eventType, string(payload))
		if err != nil {
			t.Fatalf("insert temporary event %s: %v", id, err)
		}
	}

	var active atomic.Int32
	var maximum atomic.Int32
	var handled atomic.Int32
	handler := func(ctx context.Context, _ EventInput) (json.RawMessage, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for observed := maximum.Load(); current > observed; observed = maximum.Load() {
			if maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
		handled.Add(1)
		return json.RawMessage(`{"ok":true}`), nil
	}

	processorCtx, stopProcessor := context.WithCancel(ctx)
	processor := EventProcessor{
		DB:       db,
		Handlers: map[string]EventHandler{eventType: handler},
		Interval: 10 * time.Millisecond,
		Workers:  eventCount,
		WorkerID: "concurrency-integration",
	}
	processorDone := make(chan struct{})
	go func() {
		defer close(processorDone)
		processor.Run(processorCtx)
	}()
	defer func() {
		stopProcessor()
		<-processorDone
	}()

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		var completed int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM portal.event_inbox WHERE event_id = ANY(ARRAY[$1::uuid, $2::uuid, $3::uuid, $4::uuid]) AND status = 'completed'`, ids[0], ids[1], ids[2], ids[3]).Scan(&completed); err != nil {
			stopProcessor()
			<-processorDone
			t.Fatal(err)
		}
		if completed == eventCount {
			break
		}
		select {
		case <-ctx.Done():
			stopProcessor()
			<-processorDone
			t.Fatal("timed out waiting for concurrent events to complete")
		case <-time.After(50 * time.Millisecond):
		}
	}
	stopProcessor()
	<-processorDone

	if got := handled.Load(); got != eventCount {
		t.Fatalf("handler executions = %d, want %d", got, eventCount)
	}
	if got := maximum.Load(); got < 2 {
		t.Fatalf("maximum concurrent handlers = %d, want at least 2", got)
	}
	for _, id := range ids {
		assertEventState(t, context.Background(), db, id, "completed", 1)
	}
	t.Logf("verified %d distinct events; maximum concurrent handlers=%d", eventCount, maximum.Load())
}

func newTestEventUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:]), nil
}
