package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	defaultHTTPAddr        = "127.0.0.1:8081"
	defaultSocket          = "/var/run/personal-portal/event.sock"
	defaultDatabaseURLFile = "/usr/local/etc/personal-portal/database-url"
	protocolVersion        = "v1"
)

var requestSequence atomic.Uint64

const maxEventBodyBytes = 1 << 20

var (
	eventIDPattern   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	eventTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,99}$`)
	errEventConflict = errors.New("event ID already exists with different content")
)

type EventInput struct {
	EventID   string          `json:"event_id"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
}

type EventStore interface {
	ReceiveEvent(context.Context, EventInput) (duplicate bool, err error)
}

type PostgresEventStore struct{ DB *sql.DB }

func (s PostgresEventStore) ReceiveEvent(ctx context.Context, event EventInput) (bool, error) {
	var insertedID string
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO portal.event_inbox (event_id, event_type, payload)
		VALUES ($1::uuid, $2, $3::jsonb)
		ON CONFLICT (event_id) DO NOTHING
		RETURNING event_id::text`, event.EventID, event.EventType, string(event.Payload)).Scan(&insertedID)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}

	var existingType string
	var existingPayload []byte
	err = s.DB.QueryRowContext(ctx, `
		SELECT event_type, payload::text
		FROM portal.event_inbox WHERE event_id = $1::uuid`, event.EventID).Scan(&existingType, &existingPayload)
	if err != nil {
		return false, err
	}
	var incoming, existing any
	if json.Unmarshal(event.Payload, &incoming) != nil || json.Unmarshal(existingPayload, &existing) != nil {
		return false, errEventConflict
	}
	incomingJSON, _ := json.Marshal(incoming)
	existingJSON, _ := json.Marshal(existing)
	if existingType != event.EventType || string(incomingJSON) != string(existingJSON) {
		return false, errEventConflict
	}
	return true, nil
}

func validateEvent(event EventInput) error {
	if !eventIDPattern.MatchString(event.EventID) {
		return errors.New("event_id must be a UUID")
	}
	if !eventTypePattern.MatchString(event.EventType) {
		return errors.New("event_type must start with a lowercase letter and contain only lowercase letters, digits, dot, underscore, or hyphen")
	}
	if len(event.Payload) == 0 || !json.Valid(event.Payload) {
		return errors.New("payload must be a JSON object")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload == nil {
		return errors.New("payload must be a JSON object")
	}
	return nil
}

func receiveEventHandler(store EventStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxEventBodyBytes)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var event EventInput
		if err := decoder.Decode(&event); err != nil {
			http.Error(w, "invalid event request", http.StatusBadRequest)
			return
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			http.Error(w, "invalid event request", http.StatusBadRequest)
			return
		}
		if err := validateEvent(event); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		duplicate, err := store.ReceiveEvent(ctx, event)
		if errors.Is(err, errEventConflict) {
			http.Error(w, "event_id conflicts with previously received content", http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, "event persistence unavailable", http.StatusServiceUnavailable)
			return
		}
		status := http.StatusAccepted
		if duplicate {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"event_id": event.EventID, "status": "received", "duplicate": duplicate})
	}
}

type IPCClient struct {
	SocketPath string
	Timeout    time.Duration
}

func (c IPCClient) Status(ctx context.Context) error {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = time.Second
	}

	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "unix", c.SocketPath)
	if err != nil {
		return fmt.Errorf("connect event core: %w", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set IPC deadline: %w", err)
	}

	id := fmt.Sprintf("health%d", requestSequence.Add(1))
	if _, err := fmt.Fprintf(conn, "%s %s STATUS\n", protocolVersion, id); err != nil {
		return fmt.Errorf("write IPC request: %w", err)
	}

	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return fmt.Errorf("read IPC response: %w", err)
	}
	if len(line) > 255 {
		return errors.New("IPC response exceeds limit")
	}
	fields := strings.Fields(strings.TrimSuffix(line, "\n"))
	if len(fields) != 4 || fields[0] != protocolVersion ||
		fields[1] != id || fields[2] != "OK" || fields[3] != "RUNNING" {
		return fmt.Errorf("unexpected IPC response %q", strings.TrimSpace(line))
	}
	return nil
}

func (c IPCClient) NotifyEvent(ctx context.Context, eventID, eventType string) error {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "unix", c.SocketPath)
	if err != nil {
		return fmt.Errorf("connect event core: %w", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set IPC deadline: %w", err)
	}
	id := fmt.Sprintf("evt%d", requestSequence.Add(1))
	if _, err := fmt.Fprintf(conn, "%s %s EVENT %s %s\n",
		protocolVersion, id, eventID, eventType); err != nil {
		return fmt.Errorf("write IPC event notification: %w", err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return fmt.Errorf("read IPC event response: %w", err)
	}
	if len(line) > 255 {
		return errors.New("IPC response exceeds limit")
	}
	fields := strings.Fields(strings.TrimSuffix(line, "\n"))
	if len(fields) != 4 || fields[0] != protocolVersion ||
		fields[1] != id || fields[2] != "OK" || fields[3] != "ACCEPTED" {
		return fmt.Errorf("unexpected IPC event response %q", strings.TrimSpace(line))
	}
	return nil
}

type pendingNotification struct {
	EventID   string
	EventType string
	Attempts  int
}

type EventDispatcher struct {
	DB       *sql.DB
	IPC      IPCClient
	Batch    int
	Interval time.Duration
}

func (d EventDispatcher) claim(ctx context.Context) ([]pendingNotification, error) {
	batch := d.Batch
	if batch <= 0 || batch > 64 {
		batch = 16
	}
	rows, err := d.DB.QueryContext(ctx, `
		WITH candidates AS (
			SELECT event_id
			FROM portal.event_inbox
			WHERE ((dispatch_state = 'pending' AND next_dispatch_at <= now())
			       OR (dispatch_state = 'leased' AND dispatch_lease_until < now()))
			ORDER BY next_dispatch_at, received_at, event_id
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE portal.event_inbox AS e
		SET dispatch_state = 'leased',
		    dispatch_attempts = e.dispatch_attempts + 1,
		    dispatch_lease_until = now() + interval '30 seconds',
		    dispatch_error = NULL
		FROM candidates AS c
		WHERE e.event_id = c.event_id
		RETURNING e.event_id::text, e.event_type, e.dispatch_attempts
	`, batch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []pendingNotification
	for rows.Next() {
		var event pendingNotification
		if err := rows.Scan(&event.EventID, &event.EventType, &event.Attempts); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (d EventDispatcher) markNotified(ctx context.Context, eventID string) error {
	_, err := d.DB.ExecContext(ctx, `
		UPDATE portal.event_inbox
		SET dispatch_state = 'notified', dispatch_lease_until = NULL,
		    dispatch_notified_at = now(), dispatch_error = NULL
		WHERE event_id = $1::uuid AND dispatch_state = 'leased'
	`, eventID)
	return err
}

func (d EventDispatcher) markRetry(ctx context.Context, event pendingNotification, cause error) error {
	exponent := event.Attempts - 1
	if exponent < 0 {
		exponent = 0
	}
	if exponent > 6 {
		exponent = 6
	}
	delay := time.Duration(1<<exponent) * time.Second
	message := cause.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	_, err := d.DB.ExecContext(ctx, `
		UPDATE portal.event_inbox
		SET dispatch_state = 'pending', dispatch_lease_until = NULL,
		    next_dispatch_at = now() + ($2 * interval '1 second'),
		    dispatch_error = $3
		WHERE event_id = $1::uuid AND dispatch_state = 'leased'
	`, event.EventID, delay.Seconds(), message)
	return err
}

func (d EventDispatcher) Run(ctx context.Context) {
	interval := d.Interval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		claimCtx, claimCancel := context.WithTimeout(ctx, 3*time.Second)
		events, err := d.claim(claimCtx)
		claimCancel()
		if err == nil {
			for _, event := range events {
				if ctx.Err() != nil {
					return
				}
				notifyCtx, notifyCancel := context.WithTimeout(ctx, d.IPC.Timeout)
				notifyErr := d.IPC.NotifyEvent(notifyCtx, event.EventID, event.EventType)
				notifyCancel()
				if notifyErr != nil {
					retryCtx, retryCancel := context.WithTimeout(context.Background(), 2*time.Second)
					_ = d.markRetry(retryCtx, event, notifyErr)
					retryCancel()
					continue
				}
				markCtx, markCancel := context.WithTimeout(context.Background(), 2*time.Second)
				_ = d.markNotified(markCtx, event.EventID)
				markCancel()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func loadDatabaseURL() (string, error) {
	if value := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_DATABASE_URL")); value != "" {
		return value, nil
	}
	path := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_DATABASE_URL_FILE"))
	if path == "" {
		path = defaultDatabaseURLFile
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read database URL file: %w", err)
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", errors.New("database URL file is empty")
	}
	return value, nil
}

func configuredEventProcessorWorkers() int {
	value := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_EVENT_WORKERS"))
	if value == "" {
		return defaultProcessingWorkers
	}
	workers, err := strconv.Atoi(value)
	if err != nil || workers < 1 {
		return defaultProcessingWorkers
	}
	return normalizeEventProcessorWorkers(workers)
}

func main() {
	httpAddr := os.Getenv("PERSONAL_PORTAL_HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = defaultHTTPAddr
	}
	socketPath := os.Getenv("PERSONAL_PORTAL_EVENT_SOCKET")
	if socketPath == "" {
		socketPath = defaultSocket
	}

	databaseURL, err := loadDatabaseURL()
	if err != nil {
		fmt.Fprintf(os.Stderr, "backend API database configuration failed: %v\n", err)
		os.Exit(1)
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "backend API database initialization failed\n")
		os.Exit(1)
	}
	defer db.Close()
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	ipc := IPCClient{SocketPath: socketPath, Timeout: 750 * time.Millisecond}
	go (EventDispatcher{DB: db, IPC: ipc, Batch: 16, Interval: 500 * time.Millisecond}).Run(context.Background())
	go (EventProcessor{DB: db, Handlers: registeredEventHandlers(), Interval: 500 * time.Millisecond, Workers: configuredEventProcessorWorkers()}).Run(context.Background())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("POST /events", receiveEventHandler(PostgresEventStore{DB: db}))
	mux.HandleFunc("GET /dbz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), ipc.Timeout)
		defer cancel()
		if err := ipc.Status(ctx); err != nil {
			http.Error(w, "event core unavailable", http.StatusServiceUnavailable)
			return
		}
		dbCtx, dbCancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
		defer dbCancel()
		if err := db.PingContext(dbCtx); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})

	server := &http.Server{
		Addr:              httpAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(os.Stderr, "backend API failed: %v\n", err)
		os.Exit(1)
	}
}
