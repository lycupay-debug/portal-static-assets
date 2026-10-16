package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIPCClientStatus(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "event.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		line, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			serverDone <- err
			return
		}
		var version, requestID, command string
		if _, err := fmt.Sscanf(line, "%s %s %s", &version, &requestID, &command); err != nil {
			serverDone <- err
			return
		}
		if version != protocolVersion || command != "STATUS" {
			serverDone <- fmt.Errorf("unexpected request: %q", line)
			return
		}
		_, err = fmt.Fprintf(conn, "%s %s OK RUNNING\n", version, requestID)
		serverDone <- err
	}()

	client := IPCClient{SocketPath: socketPath, Timeout: time.Second}
	if err := client.Status(context.Background()); err != nil {
		t.Fatalf("Status() returned error: %v", err)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("fake event core: %v", err)
	}
}

func TestIPCClientRejectsUnexpectedResponse(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "event.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		line, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			return
		}
		var version, requestID, command string
		if _, err := fmt.Sscanf(line, "%s %s %s", &version, &requestID, &command); err != nil {
			return
		}
		_, _ = fmt.Fprintf(conn, "%s %s ERROR UNKNOWN_COMMAND\n", version, requestID)
	}()

	client := IPCClient{SocketPath: socketPath, Timeout: time.Second}
	if err := client.Status(context.Background()); err == nil {
		t.Fatal("Status() accepted an unexpected response")
	}
}

func TestIPCClientNotifyEvent(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "event.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		line, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			serverDone <- err
			return
		}
		fields := strings.Fields(line)
		if len(fields) != 5 || fields[0] != protocolVersion ||
			fields[2] != "EVENT" ||
			fields[3] != "018f2f5e-7a2b-7c4d-8e9f-0123456789ab" ||
			fields[4] != "content.published" {
			serverDone <- fmt.Errorf("unexpected event request: %q", line)
			return
		}
		_, err = fmt.Fprintf(conn, "%s %s OK ACCEPTED\n", fields[0], fields[1])
		serverDone <- err
	}()

	client := IPCClient{SocketPath: socketPath, Timeout: time.Second}
	if err := client.NotifyEvent(context.Background(),
		"018f2f5e-7a2b-7c4d-8e9f-0123456789ab", "content.published"); err != nil {
		t.Fatalf("NotifyEvent() returned error: %v", err)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("fake event core: %v", err)
	}
}

func TestIPCClientNotifyEventRejectsUnexpectedResponse(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "event.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		line, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			return
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return
		}
		_, _ = fmt.Fprintf(conn, "%s %s OK RUNNING\n", fields[0], fields[1])
	}()
	client := IPCClient{SocketPath: socketPath, Timeout: time.Second}
	if err := client.NotifyEvent(context.Background(),
		"018f2f5e-7a2b-7c4d-8e9f-0123456789ab", "content.published"); err == nil {
		t.Fatal("NotifyEvent() accepted an unexpected response")
	}
}

func TestLoadDatabaseURLFromFile(t *testing.T) {
	t.Setenv("PERSONAL_PORTAL_DATABASE_URL", "")
	path := filepath.Join(t.TempDir(), "database-url")
	if err := os.WriteFile(path, []byte("postgres://test:secret@127.0.0.1/testdb?sslmode=disable\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PERSONAL_PORTAL_DATABASE_URL_FILE", path)
	got, err := loadDatabaseURL()
	if err != nil {
		t.Fatalf("loadDatabaseURL() error: %v", err)
	}
	if got != "postgres://test:secret@127.0.0.1/testdb?sslmode=disable" {
		t.Fatalf("loadDatabaseURL() = %q, want configured URL", got)
	}
}

func TestLoadDatabaseURLRejectsEmptyFile(t *testing.T) {
	t.Setenv("PERSONAL_PORTAL_DATABASE_URL", "")
	path := filepath.Join(t.TempDir(), "database-url")
	if err := os.WriteFile(path, []byte("  \n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PERSONAL_PORTAL_DATABASE_URL_FILE", path)
	if _, err := loadDatabaseURL(); err == nil {
		t.Fatal("loadDatabaseURL() accepted an empty file")
	}
}

type fakeEventStore struct {
	events map[string]EventInput
}

func (s *fakeEventStore) ReceiveEvent(_ context.Context, event EventInput) (bool, error) {
	if s.events == nil {
		s.events = make(map[string]EventInput)
	}
	if previous, ok := s.events[event.EventID]; ok {
		if previous.EventType != event.EventType || string(previous.Payload) != string(event.Payload) {
			return false, errEventConflict
		}
		return true, nil
	}
	s.events[event.EventID] = event
	return false, nil
}

func TestReceiveEventHandlerAcceptsAndDeduplicates(t *testing.T) {
	store := &fakeEventStore{}
	handler := receiveEventHandler(store)
	body := `{"event_id":"018f2f5e-7a2b-7c4d-8e9f-0123456789ab","event_type":"content.published","payload":{"record_id":"x"}}`

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(body)))
	if first.Code != http.StatusAccepted {
		t.Fatalf("first status = %d, want %d; body=%s", first.Code, http.StatusAccepted, first.Body.String())
	}
	if !strings.Contains(first.Body.String(), `"duplicate":false`) {
		t.Fatalf("first response should not be marked duplicate: %s", first.Body.String())
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(body)))
	if second.Code != http.StatusOK {
		t.Fatalf("duplicate status = %d, want %d; body=%s", second.Code, http.StatusOK, second.Body.String())
	}
	if !strings.Contains(second.Body.String(), `"duplicate":true`) {
		t.Fatalf("duplicate response not marked duplicate: %s", second.Body.String())
	}
}

func TestReceiveEventHandlerRejectsInvalidAndConflictingEvents(t *testing.T) {
	t.Run("invalid UUID", func(t *testing.T) {
		handler := receiveEventHandler(&fakeEventStore{})
		recorder := httptest.NewRecorder()
		body := `{"event_id":"not-a-uuid","event_type":"content.published","payload":{}}`
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", recorder.Code)
		}
	})
	t.Run("payload must be object", func(t *testing.T) {
		handler := receiveEventHandler(&fakeEventStore{})
		recorder := httptest.NewRecorder()
		body := `{"event_id":"018f2f5e-7a2b-7c4d-8e9f-0123456789ab","event_type":"content.published","payload":[]}`
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", recorder.Code)
		}
	})
	t.Run("conflicting replay", func(t *testing.T) {
		store := &fakeEventStore{}
		handler := receiveEventHandler(store)
		firstBody := `{"event_id":"018f2f5e-7a2b-7c4d-8e9f-0123456789ab","event_type":"content.published","payload":{"n":1}}`
		secondBody := `{"event_id":"018f2f5e-7a2b-7c4d-8e9f-0123456789ab","event_type":"content.deleted","payload":{"n":2}}`
		first := httptest.NewRecorder()
		handler.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(firstBody)))
		second := httptest.NewRecorder()
		handler.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(secondBody)))
		if second.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409", second.Code)
		}
	})
}

func TestConfiguredEventProcessorWorkers(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
		want  int
	}{
		{name: "unset defaults", value: "", want: defaultProcessingWorkers},
		{name: "configured", value: "2", want: 2},
		{name: "invalid defaults", value: "many", want: defaultProcessingWorkers},
		{name: "zero defaults", value: "0", want: defaultProcessingWorkers},
		{name: "negative defaults", value: "-2", want: defaultProcessingWorkers},
		{name: "upper bound", value: "99", want: maxProcessingWorkers},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("PERSONAL_PORTAL_EVENT_WORKERS", test.value)
			if got := configuredEventProcessorWorkers(); got != test.want {
				t.Fatalf("configuredEventProcessorWorkers() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestIPCSubscriberReceivesPushAndWakesWorker(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "event.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		request, err := reader.ReadString('\n')
		if err != nil {
			serverErr <- err
			return
		}
		fields := strings.Fields(strings.TrimSpace(request))
		if len(fields) != 3 || fields[0] != protocolVersion || fields[2] != "SUBSCRIBE" {
			serverErr <- fmt.Errorf("unexpected subscribe request %q", request)
			return
		}
		if _, err := fmt.Fprintf(conn, "%s %s OK SUBSCRIBED\n", protocolVersion, fields[1]); err != nil {
			serverErr <- err
			return
		}
		_, err = fmt.Fprint(conn, "v1 push1 EVENT 018f2f5e-7a2b-7c4d-8e9f-0123456789ab portal.content.validate\n")
		serverErr <- err
	}()

	wake := make(chan struct{}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client := IPCClient{SocketPath: socketPath, Timeout: time.Second}
	if err := client.subscribeOnce(ctx, wake); err == nil {
		t.Fatal("subscribeOnce unexpectedly returned nil after server closed")
	}
	select {
	case <-wake:
	case <-time.After(time.Second):
		t.Fatal("event push did not wake the worker")
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}
