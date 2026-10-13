package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"path/filepath"
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
