// ipc_client_test.go verifies bounded IPC frames and reconnect backoff limits.

package main

import (
	"bufio"
	"errors"
	"strings"
	"testing"
	"time"
)

// These tests pin the IPC client's protocol-frame and reconnect resource bounds.
func TestReadIPCLineEnforcesProtocolFrameLimit(t *testing.T) {
	valid := "v2 req1 OK RUNNING\n"
	line, err := readIPCLine(bufio.NewReaderSize(strings.NewReader(valid), maxIPCFrameBytes+1))
	if err != nil || line != valid {
		t.Fatalf("readIPCLine() = %q, %v; want %q", line, err, valid)
	}

	for _, size := range []int{maxIPCFrameBytes + 1, 1024 * 1024} {
		reader := bufio.NewReaderSize(strings.NewReader(strings.Repeat("x", size)), maxIPCFrameBytes+1)
		if _, err := readIPCLine(reader); !errors.Is(err, errIPCFrameTooLarge) {
			t.Errorf("readIPCLine(%d bytes) error = %v, want errIPCFrameTooLarge", size, err)
		}
	}
}

func TestNextIPCBackoffNeverExceedsTenSeconds(t *testing.T) {
	tests := []struct {
		current time.Duration
		want    time.Duration
	}{
		{0, time.Second},
		{time.Second, 2 * time.Second},
		{2 * time.Second, 4 * time.Second},
		{4 * time.Second, 8 * time.Second},
		{8 * time.Second, 10 * time.Second},
		{10 * time.Second, 10 * time.Second},
		{16 * time.Second, 10 * time.Second},
	}
	for _, test := range tests {
		if got := nextIPCBackoff(test.current); got != test.want {
			t.Errorf("nextIPCBackoff(%s) = %s, want %s", test.current, got, test.want)
		}
	}
}
