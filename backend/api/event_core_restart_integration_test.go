package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// This test exercises the real C daemon and Go subscriber together. It is
// intentionally FreeBSD-only because the event core uses kqueue/kevent.
func TestIPCSubscriberReconnectsAfterRealEventCoreRestart(t *testing.T) {
	if runtime.GOOS != "freebsd" {
		t.Skip("the production event core requires FreeBSD kqueue")
	}

	projectRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	tempDir := t.TempDir()
	binary := filepath.Join(tempDir, "event-core")
	socketPath := filepath.Join(tempDir, "event.sock")
	logPath := filepath.Join(tempDir, "event-core.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()

	source := filepath.Join(projectRoot, "event-core", "src", "main.c")
	includeDir := filepath.Join(projectRoot, "event-core", "include")
	build := exec.Command("cc", "-D_POSIX_C_SOURCE=200809L", "-I"+includeDir,
		"-std=c11", "-Wall", "-Wextra", "-Wpedantic", "-Werror",
		source, "-o", binary)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build event core: %v\n%s", err, output)
	}

	startCore := func() *exec.Cmd {
		cmd := exec.Command(binary, socketPath)
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		if err := cmd.Start(); err != nil {
			t.Fatalf("start event core: %v", err)
		}
		t.Cleanup(func() {
			if cmd.Process != nil {
				_ = cmd.Process.Signal(syscall.SIGTERM)
				_ = cmd.Wait()
			}
		})
		waitForCoreStatus(t, socketPath, 3*time.Second)
		return cmd
	}

	core := startCore()
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
	defer cancel()
	wake := make(chan struct{}, 8)
	client := IPCClient{SocketPath: socketPath, Timeout: 300 * time.Millisecond}
	subscriberCtx, stopSubscriber := context.WithCancel(ctx)
	defer stopSubscriber()
	go client.Subscribe(subscriberCtx, wake)

	// Establish that the first subscription is live before restarting the daemon.
	publishUntilWake(t, ctx, socketPath, wake)
	// Discard any extra pushes from the pre-restart process so the second phase
	// can only pass after a notification arrives through a fresh subscription.
	for len(wake) > 0 {
		<-wake
	}
	if err := core.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal event core: %v", err)
	}
	if err := core.Wait(); err != nil {
		t.Fatalf("wait for event core shutdown: %v", err)
	}
	core.Process = nil

	// Recreate the daemon at the same socket path. The Go subscriber must
	// detect EOF, back off, reconnect, and receive a fresh push.
	core = startCore()
	publishUntilWake(t, ctx, socketPath, wake)
	_ = core.Process.Signal(syscall.SIGTERM)
	_ = core.Wait()
	core.Process = nil
}

func waitForCoreStatus(t *testing.T, socketPath string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	client := IPCClient{SocketPath: socketPath, Timeout: 100 * time.Millisecond}
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		err := client.Status(ctx)
		cancel()
		if err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("event core did not become ready at %s", socketPath)
}

func publishUntilWake(t *testing.T, ctx context.Context, socketPath string, wake <-chan struct{}) {
	t.Helper()
	eventID := "018f2f5e-7a2b-7c4d-8e9f-0123456789ab"
	deadline := time.NewTimer(7 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		if err := publishTestEvent(socketPath, eventID, "portal.content.validate"); err == nil {
			// A successful publish can precede reconnect completion; keep publishing
			// until the subscriber actually signals the Go worker.
		}
		select {
		case <-wake:
			return
		case <-ctx.Done():
			t.Fatalf("timed out waiting for subscriber push: %v", ctx.Err())
		case <-deadline.C:
			t.Fatal("subscriber did not receive a push from the running C event core")
		case <-ticker.C:
		}
	}
}

func publishTestEvent(socketPath, eventID, eventType string) error {
	conn, err := net.DialTimeout("unix", socketPath, 150*time.Millisecond)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(200 * time.Millisecond))
	if _, err := fmt.Fprintf(conn, "%s restarttest PUBLISH %s %s\n", protocolVersion, eventID, eventType); err != nil {
		return err
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(line) != protocolVersion+" restarttest OK PUBLISHED" {
		return fmt.Errorf("unexpected publish response %q", strings.TrimSpace(line))
	}
	return nil
}
