package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadSystemLogReturnsLatestBoundedTailAndCursor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "messages")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\nfour\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := readSystemLog(path, "server", 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || result.Items[0].Message != "three" || result.Items[1].Message != "four" {
		t.Fatalf("unexpected tail: %#v", result.Items)
	}
	if result.Cursor != int64(len("one\ntwo\nthree\nfour\n")) {
		t.Fatalf("cursor=%d", result.Cursor)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("five\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	incremental, err := readSystemLog(path, "server", result.Cursor, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(incremental.Items) != 1 || incremental.Items[0].Message != "five" {
		t.Fatalf("unexpected incremental entries: %#v", incremental.Items)
	}
}

func TestReadSystemLogHandlesLargeBoundedTailWithoutLosingCursor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "messages")
	var input strings.Builder
	for i := 0; i < 5000; i++ {
		fmt.Fprintf(&input, "entry-%04d\n", i)
	}
	if err := os.WriteFile(path, []byte(input.String()), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := readSystemLog(path, "server", 0, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 5 || result.Items[4].Message != "entry-4999" {
		t.Fatalf("unexpected large-log tail: %#v", result.Items)
	}
	if result.Cursor != int64(input.Len()) {
		t.Fatalf("cursor=%d, want %d", result.Cursor, input.Len())
	}
}

func TestParseSystemSSHLogKeepsSanitizedMetadata(t *testing.T) {
	line := `{"time":"2026-10-18T04:00:00+0000","action":"execute_shell_result","outcome":"failed","details":{"returncode":2,"timeout_seconds":10}}`
	entry := parseSystemLogLine("ssh", 42, line)
	if entry.Action != "execute_shell_result" || entry.Level != "error" || entry.Details["returncode"] != float64(2) {
		t.Fatalf("unexpected parsed entry: %#v", entry)
	}
}

func TestParseGoSSHLogAddsChineseSourceAndOperationLabels(t *testing.T) {
	line := `{"time":"2026-10-19T08:00:00Z","action":"task.started","outcome":"started","details":{"tool_source":"Go SSH MCP","operation":"shell.exec","state":"running"}}`
	entry := parseSystemLogLine("ssh", 43, line)
	if entry.Action != "Go SSH MCP · 执行 Shell 命令" || entry.Source != "ssh" {
		t.Fatalf("unexpected Go SSH MCP start entry: %#v", entry)
	}
	if got := goSSHLogAction("shell.exec", "task.completed", "success"); got != "Go SSH MCP · Shell 命令执行结果" {
		t.Fatalf("unexpected shell result label: %q", got)
	}
	if got := goSSHLogAction("file.write", "task.completed", "success"); got != "Go SSH MCP · 写入文件" {
		t.Fatalf("unexpected file write label: %q", got)
	}
}

func TestActionableSystemWarningFiltersNoiseAndKeepsCurrentIncidents(t *testing.T) {
	now := time.Date(2026, 10, 18, 5, 0, 0, 0, time.Local)
	cases := []struct {
		name  string
		entry systemLogEntry
		want  bool
	}{
		{name: "current database error", entry: systemLogEntry{Source: "server", Level: "error", Time: now.Format("Jan _2 15:04:05"), Message: "ERROR: permission denied for schema portal"}, want: true},
		{name: "retired table reference", entry: systemLogEntry{Source: "server", Level: "error", Time: now.Format("Jan _2 15:04:05"), Message: `ERROR: relation "portal.uploaded_image_assets" does not exist`}, want: false},
		{name: "optional host service config warning", entry: systemLogEntry{Source: "server", Level: "warning", Time: now.Format("Jan _2 15:04:05"), Message: "/usr/sbin/service: WARNING: $sendmail_enable is not set properly - see rc.conf(5)."}, want: false},
		{name: "test fixture error", entry: systemLogEntry{Source: "server", Level: "error", Time: now.Format("Jan _2 15:04:05"), Message: "INSERT INTO portal.event_inbox event_type='test.processor.recovery'"}, want: false},
		{name: "stale server error", entry: systemLogEntry{Source: "server", Level: "error", Time: now.Add(-2 * time.Hour).Format("Jan _2 15:04:05"), Message: "ERROR: service operation failed"}, want: false},
		{name: "current server error", entry: systemLogEntry{Source: "server", Level: "error", Time: now.Add(-5 * time.Minute).Format("Jan _2 15:04:05"), Message: "kernel: fatal service error"}, want: true},
		{name: "failed read-only SSH query", entry: systemLogEntry{Source: "ssh", Level: "error", Action: "execute_shell_result", Message: "execute_shell_result · failed"}, want: false},
		{name: "failed file mutation", entry: systemLogEntry{Source: "ssh", Level: "error", Time: now.Format("2006-01-02T15:04:05-0700"), Action: "write_file", Message: "write_file · failed"}, want: true},
		{name: "stale failed file mutation", entry: systemLogEntry{Source: "ssh", Level: "error", Time: now.Add(-2 * time.Hour).Format("2006-01-02T15:04:05-0700"), Action: "write_file", Message: "write_file · failed"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := actionableSystemWarning(tc.entry, now); got != tc.want {
				t.Fatalf("actionableSystemWarning()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseServerLogTimeConvertsEmbeddedPostgresUTCToShanghai(t *testing.T) {
	line := "Oct 26 15:53:13 freebsd postgres[23616]: [8-1] 2026-10-26 15:53:13.281 UTC [23616] FATAL: password authentication failed"
	got := parseServerLogTime(line)
	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("parseServerLogTime()=%q is not RFC3339: %v", got, err)
	}
	if parsed.UTC().Format("2006-01-02 15:04:05") != "2026-10-26 15:53:13" {
		t.Fatalf("timestamp instant changed: got %s", parsed.UTC().Format(time.RFC3339))
	}
	if parsed.Format("2006-01-02 15:04:05 -0700") != "2026-10-26 23:53:13 +0800" {
		t.Fatalf("timestamp was not displayed in Shanghai time: got %s", parsed.Format("2006-01-02 15:04:05 -0700"))
	}
}

func TestParseServerLogTimeConvertsLegacyUTCOnlySyslogPrefix(t *testing.T) {
	cutover := filepath.Join(t.TempDir(), "timezone-cutover")
	if err := os.WriteFile(cutover, []byte("2026-10-27T00:44:12+08:00\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got := parseServerLogTimeWithCutover("Oct 26 16:43:00 freebsd kernel: fatal service error", cutover)
	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("parseServerLogTime()=%q is not RFC3339: %v", got, err)
	}
	if parsed.Format("2006-01-02 15:04:05 -0700") != "2026-10-27 00:43:00 +0800" {
		t.Fatalf("legacy UTC syslog prefix was not converted: got %s", parsed.Format("2006-01-02 15:04:05 -0700"))
	}
}
