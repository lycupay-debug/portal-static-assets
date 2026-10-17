package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestServeMP3PreviewSupportsByteRanges(t *testing.T) {
	content := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	path := t.TempDir() + "/audio.mp3"
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	req := httptest.NewRequest(http.MethodGet, "/preview", nil)
	req.Header.Set("Range", "bytes=4-9")
	rec := httptest.NewRecorder()
	serveMP3Preview(rec, req, f, "课程 音频.mp3")

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d; body=%q", rec.Code, http.StatusPartialContent, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Range"); got != "bytes 4-9/36" {
		t.Fatalf("Content-Range = %q", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "audio/mpeg" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "inline; filename*=") {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), content[4:10]) {
		t.Fatalf("range body = %q, want %q", rec.Body.Bytes(), content[4:10])
	}
}

func TestServeMP3PreviewRejectsUnsatisfiableRange(t *testing.T) {
	path := t.TempDir() + "/audio.mp3"
	if err := os.WriteFile(path, []byte("short"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	req := httptest.NewRequest(http.MethodGet, "/preview", nil)
	req.Header.Set("Range", "bytes=100-200")
	rec := httptest.NewRecorder()
	serveMP3Preview(rec, req, f, "audio.mp3")
	if rec.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestedRangeNotSatisfiable)
	}
}
