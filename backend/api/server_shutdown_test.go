package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestServeUntilShutdownGracefullyStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	server := &http.Server{
		Addr:    "127.0.0.1:0",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }),
	}
	result := make(chan error, 1)
	go func() {
		result <- serveUntilShutdown(ctx, server)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("serveUntilShutdown returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP server did not shut down after context cancellation")
	}
}
