package main

import (
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		closeErr := server.Close()
		serveErr := <-serveResult
		if errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintf(os.Stderr, "backend API graceful shutdown reached 10s deadline; active connections were force-closed: %v\n", err)
			if errors.Is(serveErr, http.ErrServerClosed) || serveErr == nil {
				return nil
			}
			return serveErr
		}
		if closeErr != nil {
			return fmt.Errorf("graceful HTTP shutdown failed (%v), force close failed: %w", err, closeErr)
		}
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
		BaseContext: func(net.Listener) context.Context {
			return rootCtx
		},
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
