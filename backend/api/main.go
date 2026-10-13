package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

const (
	defaultHTTPAddr = "127.0.0.1:8081"
	defaultSocket   = "/var/run/personal-portal/event.sock"
	protocolVersion = "v1"
)

var requestSequence atomic.Uint64

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

func main() {
	httpAddr := os.Getenv("PERSONAL_PORTAL_HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = defaultHTTPAddr
	}
	socketPath := os.Getenv("PERSONAL_PORTAL_EVENT_SOCKET")
	if socketPath == "" {
		socketPath = defaultSocket
	}

	ipc := IPCClient{SocketPath: socketPath, Timeout: 750 * time.Millisecond}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
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
