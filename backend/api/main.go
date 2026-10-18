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
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	defaultHTTPAddr        = "127.0.0.1:8081"
	defaultSocket          = "/var/run/personal-portal/event.sock"
	defaultDatabaseURLFile = "/usr/local/etc/personal-portal/database-url"
	protocolVersion        = "v2"
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
	var payloadMatches bool
	err = s.DB.QueryRowContext(ctx, `
		SELECT event_type, payload = $2::jsonb
		FROM portal.event_inbox WHERE event_id = $1::uuid`, event.EventID, string(event.Payload)).Scan(&existingType, &payloadMatches)
	if err != nil {
		return false, err
	}
	if existingType != event.EventType || !payloadMatches {
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
			writeError(w, http.StatusBadRequest, "invalid event request", nil)
			return
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid event request", nil)
			return
		}
		if err := validateEvent(event); err != nil {
			writeError(w, http.StatusBadRequest, err.Error(), nil)
			return
		}
		// Privileged command-task events are internal-only. Public event intake must
		// never be able to create or wake a root command by choosing an event type.
		if event.EventType == commandTaskEventType {
			writeError(w, http.StatusForbidden, "privileged command events are internal-only", nil)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		duplicate, err := store.ReceiveEvent(ctx, event)
		if errors.Is(err, errEventConflict) {
			writeError(w, http.StatusConflict, "event_id conflicts with previously received content", nil)
			return
		}
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "event persistence unavailable", nil)
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

// Subscribe keeps a stream open and converts best-effort C-core push messages
// into worker wakeups. PostgreSQL remains authoritative; callers must retain
// the processor recovery scan because notifications can be missed on reconnect.
func (c IPCClient) Subscribe(ctx context.Context, wake chan<- struct{}) {
	backoff := time.Second
	var reconnects uint64
	for ctx.Err() == nil {
		started := time.Now()
		err := c.subscribeOnce(ctx, wake)
		if ctx.Err() != nil {
			return
		}
		reconnects++
		fmt.Printf("event_core_subscriber_disconnected reconnect=%d connection_duration=%s backoff=%s error=%v\n", reconnects, time.Since(started).Round(time.Millisecond), backoff, err)
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if backoff < 10*time.Second {
			backoff *= 2
		}
	}
}

func (c IPCClient) subscribeOnce(ctx context.Context, wake chan<- struct{}) error {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = time.Second
	}
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "unix", c.SocketPath)
	if err != nil {
		return fmt.Errorf("connect event core subscriber: %w", err)
	}
	defer conn.Close()
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	id := fmt.Sprintf("sub%d", requestSequence.Add(1))
	if _, err := fmt.Fprintf(conn, "%s %s SUBSCRIBE\n", protocolVersion, id); err != nil {
		return fmt.Errorf("send subscription request: %w", err)
	}
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read subscription acknowledgement: %w", err)
	}
	fields := strings.Fields(strings.TrimSuffix(line, "\n"))
	if len(fields) != 4 || fields[0] != protocolVersion || fields[1] != id || fields[2] != "OK" || fields[3] != "SUBSCRIBED" {
		return fmt.Errorf("unexpected subscription response %q", strings.TrimSpace(line))
	}
	fmt.Printf("event_core_subscriber_connected request_id=%s\n", id)
	// A persistent subscription cannot use the short request deadline.
	_ = conn.SetDeadline(time.Time{})
	for {
		line, err = reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read event push: %w", err)
		}
		if len(line) > 255 {
			return errors.New("event push exceeds protocol limit")
		}
		fields = strings.Fields(strings.TrimSuffix(line, "\n"))
		if len(fields) != 5 || fields[0] != protocolVersion || !validIPCRequestID(fields[1]) ||
			fields[2] != "EVENT" || !eventIDPattern.MatchString(fields[3]) || !eventTypePattern.MatchString(fields[4]) {
			return fmt.Errorf("invalid event push %q", strings.TrimSpace(line))
		}
		eventID, eventType := fields[3], fields[4]
		fmt.Printf("event_core_push_received event_id=%s event_type=%s\n", eventID, eventType)
		if wake != nil {
			select {
			case wake <- struct{}{}:
			default:
			}
		}
		ackID := fmt.Sprintf("ack%d", requestSequence.Add(1))
		if _, err := fmt.Fprintf(conn, "%s %s ACK %s %s\n", protocolVersion, ackID, eventID, eventType); err != nil {
			return fmt.Errorf("send event push acknowledgement: %w", err)
		}
		ackLine, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read event push acknowledgement: %w", err)
		}
		ackFields := strings.Fields(strings.TrimSuffix(ackLine, "\n"))
		if len(ackFields) != 4 || ackFields[0] != protocolVersion || ackFields[1] != ackID || ackFields[2] != "OK" || ackFields[3] != "ACKNOWLEDGED" {
			return fmt.Errorf("unexpected event push acknowledgement %q", strings.TrimSpace(ackLine))
		}
		fmt.Printf("event_core_push_acknowledged event_id=%s event_type=%s\n", eventID, eventType)
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

func validIPCRequestID(value string) bool {
	if len(value) == 0 || len(value) > 31 {
		return false
	}
	for _, ch := range value {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

func (c IPCClient) PublishEvent(ctx context.Context, eventID, eventType string) error {
	return c.sendEventCommand(ctx, "PUBLISH", eventID, eventType, "PUBLISHED")
}

func (c IPCClient) NotifyEvent(ctx context.Context, eventID, eventType string) error {
	return c.sendEventCommand(ctx, "EVENT", eventID, eventType, "ACCEPTED")
}

func (c IPCClient) sendEventCommand(ctx context.Context, command, eventID, eventType, expected string) error {
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
	if _, err := fmt.Fprintf(conn, "%s %s %s %s %s\n",
		protocolVersion, id, command, eventID, eventType); err != nil {
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
		fields[1] != id || fields[2] != "OK" || fields[3] != expected {
		return fmt.Errorf("unexpected IPC %s response %q", command, strings.TrimSpace(line))
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
	Wake     chan<- struct{}
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

func (d EventDispatcher) markNotified(ctx context.Context, event pendingNotification) error {
	result, err := d.DB.ExecContext(ctx, `
		UPDATE portal.event_inbox
		SET dispatch_state = 'notified', dispatch_lease_until = NULL,
		    dispatch_notified_at = now(), dispatch_error = NULL
		WHERE event_id = $1::uuid AND dispatch_state = 'leased' AND dispatch_attempts = $2
	`, event.EventID, event.Attempts)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("event %s dispatch lease was lost before notification update", event.EventID)
	}
	return nil
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
	result, err := d.DB.ExecContext(ctx, `
		UPDATE portal.event_inbox
		SET dispatch_state = 'pending', dispatch_lease_until = NULL,
		    next_dispatch_at = now() + ($2 * interval '1 second'),
		    dispatch_error = $3
		WHERE event_id = $1::uuid AND dispatch_state = 'leased' AND dispatch_attempts = $4
	`, event.EventID, delay.Seconds(), message, event.Attempts)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("event %s dispatch lease was lost before retry update", event.EventID)
	}
	return nil
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
		if err != nil {
			fmt.Printf("event_dispatcher_claim_failed error=%v\n", err)
		}
		if err == nil {
			for _, event := range events {
				notifyStarted := time.Now()
				if ctx.Err() != nil {
					return
				}
				notifyCtx, notifyCancel := context.WithTimeout(ctx, d.IPC.Timeout)
				notifyErr := d.IPC.PublishEvent(notifyCtx, event.EventID, event.EventType)
				notifyCancel()
				if notifyErr != nil {
					retryCtx, retryCancel := context.WithTimeout(context.Background(), 2*time.Second)
					retryErr := d.markRetry(retryCtx, event, notifyErr)
					retryCancel()
					fmt.Printf("event_dispatch_failed event_id=%s event_type=%s attempt=%d duration=%s error=%v retry_state_error=%v\n", event.EventID, event.EventType, event.Attempts, time.Since(notifyStarted).Round(time.Millisecond), notifyErr, retryErr)
					continue
				}
				markCtx, markCancel := context.WithTimeout(context.Background(), 2*time.Second)
				markErr := d.markNotified(markCtx, event)
				markCancel()
				if markErr != nil {
					fmt.Printf("event_dispatch_state_update_failed event_id=%s error=%v\n", event.EventID, markErr)
				} else {
					fmt.Printf("event_dispatch_notified event_id=%s event_type=%s attempt=%d duration=%s\n", event.EventID, event.EventType, event.Attempts, time.Since(notifyStarted).Round(time.Millisecond))
				}
				// A successful C-core acknowledgement is the fast-path wakeup.
				// Wakeups are hints only; PostgreSQL leases and recovery scans
				// guarantee that a dropped signal cannot lose a persisted event.
				if d.Wake != nil {
					select {
					case d.Wake <- struct{}{}:
					default:
					}
				}
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

func serveUntilShutdown(ctx context.Context, server *http.Server) error {
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
	case err := <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("graceful HTTP shutdown: %w", err)
	}
	err := <-serveResult
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
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

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if len(os.Args) > 1 && os.Args[1] == "--task-executor" {
		if os.Geteuid() != 0 {
			fmt.Fprintln(os.Stderr, "privileged task executor must run as root")
			os.Exit(1)
		}
		runCommandTaskExecutor(rootCtx, db)
		return
	}

	ipc := IPCClient{SocketPath: socketPath, Timeout: 750 * time.Millisecond}
	// The C core acknowledgement wakes Go workers immediately. This buffered
	// channel is intentionally lossy: the database is durable and periodic
	// recovery scans reclaim work if a signal is missed or a process restarts.
	eventWake := make(chan struct{}, 256)
	var background sync.WaitGroup
	background.Add(3)
	go func() {
		defer background.Done()
		(EventDispatcher{DB: db, IPC: ipc, Batch: 16, Interval: 5 * time.Second, Wake: eventWake}).Run(rootCtx)
	}()
	go func() {
		defer background.Done()
		ipc.Subscribe(rootCtx, eventWake)
	}()
	go func() {
		defer background.Done()
		(EventProcessor{DB: db, Handlers: registeredEventHandlers(), Interval: 5 * time.Second, Workers: configuredEventProcessorWorkers(), Wake: eventWake}).Run(rootCtx)
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("GET /metrics", eventMetricsHandler(db))
	mux.HandleFunc("POST /events", receiveEventHandler(PostgresEventStore{DB: db}))
	mux.Handle("/", newPortalAPI(db, ipc))
	mux.HandleFunc("GET /dbz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			writeError(w, http.StatusServiceUnavailable, "database unavailable", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), ipc.Timeout)
		defer cancel()
		if err := ipc.Status(ctx); err != nil {
			writeError(w, http.StatusServiceUnavailable, "event core unavailable", nil)
			return
		}
		dbCtx, dbCancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
		defer dbCancel()
		if err := db.PingContext(dbCtx); err != nil {
			writeError(w, http.StatusServiceUnavailable, "database unavailable", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// SSE streams need to stay open; retain a write deadline for ordinary API responses.
		if r.URL.Path != "/system-ops/stream" {
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
		}
		mux.ServeHTTP(w, r)
	})
	server := &http.Server{
		Addr:              httpAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}

	serverErr := serveUntilShutdown(rootCtx, server)
	if serverErr != nil {
		fmt.Fprintf(os.Stderr, "backend API failed: %v\n", serverErr)
	}
	stop()
	backgroundDone := make(chan struct{})
	go func() {
		background.Wait()
		close(backgroundDone)
	}()
	select {
	case <-backgroundDone:
	case <-time.After(5 * time.Second):
		fmt.Fprintln(os.Stderr, "backend API background workers did not stop within 5 seconds")
	}
	if serverErr != nil {
		os.Exit(1)
	}
}
