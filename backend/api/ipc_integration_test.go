// ipc_integration_test.go exercises the Go client against a real native C event-core process.

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

type eventCoreProcess struct {
	command *exec.Cmd
	done    chan error
	stopped bool
}

// TestIPCClientAgainstRealEventCore runs only when the CI event-core job supplies
// a native kqueue-enabled C binary. It verifies the Go/C wire contract and that
// the existing subscriber reconnects after the C process is stopped and restarted.
func TestIPCClientAgainstRealEventCore(t *testing.T) {
	binary := os.Getenv("PORTAL_EVENT_CORE_BINARY")
	if binary == "" {
		t.Skip("PORTAL_EVENT_CORE_BINARY is not set; native Go/C IPC integration is disabled")
	}
	if info, err := os.Stat(binary); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("event-core binary is unavailable: %q (%v)", binary, err)
	}

	// sockaddr_un has a short platform-specific path limit (104 bytes on macOS).
	// t.TempDir() inherits the deep GitHub runner workspace path, so keep the
	// socket under /tmp to make the native kqueue integration portable.
	tempDir, err := os.MkdirTemp("/tmp", "ppipc-")
	if err != nil {
		t.Fatalf("create short IPC temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)
	socketPath := filepath.Join(tempDir, "e.sock")
	if len(socketPath) >= 100 {
		t.Fatalf("temporary IPC socket path is too long for sockaddr_un: %s", socketPath)
	}
	client := IPCClient{SocketPath: socketPath, Timeout: 750 * time.Millisecond}

	stopCore := func(core *eventCoreProcess) {
		t.Helper()
		if core == nil || core.command == nil || core.command.Process == nil || core.stopped {
			return
		}
		if core.command.ProcessState != nil {
			core.stopped = true
			return
		}
		if err := core.command.Process.Signal(syscall.SIGTERM); err != nil && err != os.ErrProcessDone {
			t.Errorf("signal event core: %v", err)
		}
		select {
		case err := <-core.done:
			if err != nil {
				t.Errorf("event core exited with error: %v", err)
			}
		case <-time.After(3 * time.Second):
			_ = core.command.Process.Kill()
			<-core.done
			t.Error("event core did not stop after SIGTERM")
		}
		core.stopped = true
	}

	startCore := func() *eventCoreProcess {
		t.Helper()
		command := exec.Command(binary, socketPath)
		command.Stdout = os.Stderr
		command.Stderr = os.Stderr
		if err := command.Start(); err != nil {
			t.Fatalf("start event core: %v", err)
		}
		core := &eventCoreProcess{command: command, done: make(chan error, 1)}
		go func() { core.done <- command.Wait() }()
		t.Cleanup(func() { stopCore(core) })

		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			select {
			case err := <-core.done:
				t.Fatalf("event core exited before readiness: %v", err)
			default:
			}
			ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			err := client.Status(ctx)
			cancel()
			if err == nil {
				return core
			}
			time.Sleep(25 * time.Millisecond)
		}
		t.Fatalf("event core did not become ready at %s", socketPath)
		return nil
	}

	core := startCore()
	subCtx, cancelSub := context.WithCancel(context.Background())
	wake := make(chan struct{}, 1)
	subscriberDone := make(chan struct{})
	go func() {
		defer close(subscriberDone)
		client.Subscribe(subCtx, wake)
	}()

	publishUntilWake := func(label string) {
		t.Helper()
		deadline := time.NewTimer(8 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-wake:
				return
			case <-deadline.C:
				t.Fatalf("subscriber did not receive %s event before deadline", label)
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				err := client.PublishEvent(ctx, randomUUID(), "portal.test.ipc.integration")
				cancel()
				if err != nil && subCtx.Err() == nil {
					t.Logf("publish while subscriber reconnects: %v", err)
				}
			}
		}
	}

	publishUntilWake("initial")
	// Let the subscriber finish ACKing the notification, then remove any
	// coalesced wakeups before stopping the C process for the reconnect case.
	time.Sleep(100 * time.Millisecond)
	for {
		select {
		case <-wake:
		default:
			goto wakeQueueDrained
		}
	}

wakeQueueDrained:
	stopCore(core)
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Fatalf("socket path still exists after core shutdown: %v", err)
	}
	core = startCore()
	publishUntilWake("post-restart")

	cancelSub()
	select {
	case <-subscriberDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Go subscriber did not exit after context cancellation")
	}
	stopCore(core)
}
