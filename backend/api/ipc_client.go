package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

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
