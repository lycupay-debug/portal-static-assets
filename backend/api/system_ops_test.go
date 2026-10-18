package main

import (
	"os"
	"path/filepath"
	"testing"
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
