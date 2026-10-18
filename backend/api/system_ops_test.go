package main

import (
	"os"
	"path/filepath"
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

func TestParseSystemSSHLogKeepsSanitizedMetadata(t *testing.T) {
	line := `{"time":"2026-10-18T04:00:00+0000","action":"execute_shell_result","outcome":"failed","details":{"returncode":2,"timeout_seconds":10}}`
	entry := parseSystemLogLine("ssh", 42, line)
	if entry.Action != "execute_shell_result" || entry.Level != "error" || entry.Details["returncode"] != float64(2) {
		t.Fatalf("unexpected parsed entry: %#v", entry)
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
		{name: "manual probe against retired database", entry: systemLogEntry{Source: "server", Level: "error", Time: now.Format("Jan _2 15:04:05"), Message: `postgres: FATAL: database "app_core" does not exist`}, want: false},
		{name: "manual probe using local root role", entry: systemLogEntry{Source: "server", Level: "error", Time: now.Format("Jan _2 15:04:05"), Message: `postgres: FATAL: role "root" does not exist`}, want: false},
		{name: "invalid diagnostic catalog column", entry: systemLogEntry{Source: "server", Level: "error", Time: now.Format("Jan _2 15:04:05"), Message: `postgres: ERROR: column "idx_tup_read" does not exist`}, want: false},
		{name: "invalid diagnostic index expression", entry: systemLogEntry{Source: "server", Level: "error", Time: now.Format("Jan _2 15:04:05"), Message: "postgres: ERROR: functions in index expression must be marked IMMUTABLE"}, want: false},
		{name: "unknown production database error remains actionable", entry: systemLogEntry{Source: "server", Level: "error", Time: now.Format("Jan _2 15:04:05"), Message: "postgres: ERROR: permission denied for schema portal"}, want: true},
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
