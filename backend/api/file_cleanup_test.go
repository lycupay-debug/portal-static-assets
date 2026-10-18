package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFileCleanupHandlerRemovesManagedFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PERSONAL_PORTAL_UPLOAD_DIR", dir)
	filename := randomUUID() + ".png"
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte("test image"), 0600); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(fileCleanupPayload{Kind: "upload_image", Filename: filename})
	if err != nil {
		t.Fatal(err)
	}
	result, err := fileCleanupEventHandler(context.Background(), EventInput{Payload: payload})
	if err != nil {
		t.Fatalf("cleanup handler returned error: %v", err)
	}
	if string(result) != `{"removed":true}` {
		t.Fatalf("result = %s", result)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("managed file remains or stat failed unexpectedly: %v", err)
	}
}

func TestFileCleanupHandlerMissingFileIsIdempotent(t *testing.T) {
	t.Setenv("PERSONAL_PORTAL_RESOURCE_DIR", t.TempDir())
	payload, err := json.Marshal(fileCleanupPayload{Kind: "teaching_resource", Filename: randomUUID() + ".pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fileCleanupEventHandler(context.Background(), EventInput{Payload: payload}); err != nil {
		t.Fatalf("missing file should be treated as success: %v", err)
	}
}

func TestFileCleanupHandlerRejectsUnsafePayload(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "path traversal", payload: `{"kind":"upload_image","filename":"../secret.png"}`},
		{name: "unsupported kind", payload: `{"kind":"arbitrary_path","filename":"` + randomUUID() + `.png"}`},
		{name: "unknown field", payload: `{"kind":"upload_image","filename":"` + randomUUID() + `.png","path":"/etc/passwd"}`},
		{name: "invalid json", payload: `[]`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := fileCleanupEventHandler(context.Background(), EventInput{Payload: []byte(test.payload)}); err == nil {
				t.Fatal("expected payload validation error")
			}
		})
	}
}
