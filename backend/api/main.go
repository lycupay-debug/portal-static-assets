package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	defaultHTTPAddr        = "127.0.0.1:8081"
	defaultDatabaseURLFile = "/usr/local/etc/personal-portal/database-url"
)

func loadDatabaseURL() (string, error) {
	if value := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_DATABASE_URL")); value != "" {
		return value, nil
	}
	path := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_DATABASE_URL_FILE"))
	if path == "" {
		path = defaultDatabaseURLFile
	}
	cleanPath := filepath.Clean(path)
	root, err := os.OpenRoot(filepath.Dir(cleanPath))
	if err != nil {
		return "", fmt.Errorf("open database URL directory: %w", err)
	}
	f, err := root.Open(filepath.Base(cleanPath))
	rootErr := root.Close()
	if err != nil {
		return "", fmt.Errorf("read database URL file: %w", err)
	}
	if rootErr != nil {
		_ = f.Close()
		return "", fmt.Errorf("close database URL directory: %w", rootErr)
	}
	data, readErr := io.ReadAll(f)
	closeErr := f.Close()
	if readErr != nil {
		return "", fmt.Errorf("read database URL file: %w", readErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close database URL file: %w", closeErr)
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
