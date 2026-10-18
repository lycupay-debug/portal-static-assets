package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const fileCleanupEventType = "portal.file.cleanup"

var managedCleanupFilename = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\.[A-Za-z0-9]{1,10}$`)

type fileCleanupPayload struct {
	Kind     string `json:"kind"`
	Filename string `json:"filename"`
}

// enqueueFileCleanup must be called with the business deletion transaction.
// The event and the deletion therefore either commit together or roll back together.
func enqueueFileCleanup(ctx context.Context, tx *sql.Tx, kind, filename string) (string, error) {
	if kind != "upload_image" && kind != "teaching_resource" {
		return "", fmt.Errorf("unsupported managed file cleanup kind %q", kind)
	}
	if !managedCleanupFilename.MatchString(filename) || filepath.Base(filename) != filename {
		return "", fmt.Errorf("invalid managed file cleanup filename")
	}
	payload, err := json.Marshal(fileCleanupPayload{Kind: kind, Filename: filename})
	if err != nil {
		return "", fmt.Errorf("encode file cleanup event: %w", err)
	}
	eventID := randomUUID()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO portal.event_inbox(event_id,event_type,payload) VALUES($1::uuid,$2,$3::jsonb)`,
		eventID, fileCleanupEventType, string(payload)); err != nil {
		return "", fmt.Errorf("persist file cleanup event: %w", err)
	}
	return eventID, nil
}

func fileCleanupEventHandler(ctx context.Context, event EventInput) (json.RawMessage, error) {
	var payload fileCleanupPayload
	decoder := json.NewDecoder(strings.NewReader(string(event.Payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode file cleanup payload: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("file cleanup payload must contain exactly one JSON object")
	}
	if !managedCleanupFilename.MatchString(payload.Filename) || filepath.Base(payload.Filename) != payload.Filename {
		return nil, errors.New("file cleanup filename failed validation")
	}

	var dir string
	switch payload.Kind {
	case "upload_image":
		dir = strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_UPLOAD_DIR"))
		if dir == "" {
			dir = "/usr/local/www/personal-portal/uploads"
		}
	case "teaching_resource":
		dir = strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_RESOURCE_DIR"))
		if dir == "" {
			dir = "/var/db/personal-portal/resources"
		}
	default:
		return nil, fmt.Errorf("unsupported file cleanup kind %q", payload.Kind)
	}
	if !filepath.IsAbs(dir) {
		return nil, errors.New("configured file cleanup directory must be absolute")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path := filepath.Join(filepath.Clean(dir), payload.Filename)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove managed file %q: %w", payload.Filename, err)
	}
	return json.RawMessage(`{"removed":true}`), nil
}
