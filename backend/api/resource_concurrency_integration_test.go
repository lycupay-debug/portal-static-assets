// resource_concurrency_integration_test.go verifies serialized file replacement and durable cleanup.

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResourcePatchRepairsMissingStoredFile(t *testing.T) {
	db := openPortalIntegrationDB(t)
	ownerID, ownerToken := insertSecurityTestUser(t, db, "resource-repair-"+strings.ReplaceAll(randomUUID(), "-", "")+"@example.invalid")
	resourceDir := t.TempDir()
	t.Setenv("PERSONAL_PORTAL_RESOURCE_DIR", resourceDir)
	api := &portalAPI{db: db, resourceDir: resourceDir, searchCache: newSearchResultCache(), ipc: IPCClient{SocketPath: filepath.Join(t.TempDir(), "no-event-core.sock")}}

	missingFilename := randomUUID() + ".pdf"
	var resourceID string
	if err := db.QueryRow(`INSERT INTO portal.teaching_resources(owner_id,title,description,resource_type,grade,is_public,original_filename,stored_filename,mime_type,file_size) VALUES($1::uuid,'Repair resource','','其他',NULL,FALSE,'same-name.pdf',$2,'application/pdf',1) RETURNING resource_id::text`, ownerID, missingFilename).Scan(&resourceID); err != nil {
		t.Fatalf("insert resource with missing file: %v", err)
	}
	var replacementFilename string
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM portal.teaching_resources WHERE resource_id=$1::uuid`, resourceID)
		_, _ = db.Exec(`DELETE FROM portal.event_inbox WHERE event_type=$1 AND payload->>'filename'=$2`, fileCleanupEventType, missingFilename)
		if replacementFilename != "" {
			_ = os.Remove(filepath.Join(resourceDir, replacementFilename))
		}
	})

	fileData := []byte("%PDF-1.7\nreplacement for missing original")
	body, err := json.Marshal(map[string]any{
		"title":       "Repaired resource",
		"description": "",
		"type":        "其他",
		"grade":       "",
		"url":         "",
		"filename":    "same-name.pdf",
		"data":        base64.StdEncoding.EncodeToString(fileData),
	})
	if err != nil {
		t.Fatal(err)
	}
	response := securityTestRequest(api, http.MethodPatch, "/resources/"+resourceID, ownerToken, string(body))
	if response.Code != http.StatusOK {
		t.Fatalf("repair PATCH returned %d: %s", response.Code, response.Body.String())
	}
	if err := db.QueryRow(`SELECT stored_filename FROM portal.teaching_resources WHERE resource_id=$1::uuid`, resourceID).Scan(&replacementFilename); err != nil {
		t.Fatalf("read repaired filename: %v", err)
	}
	if replacementFilename == missingFilename {
		t.Fatal("repair retained the missing stored filename")
	}
	if _, err := os.Stat(filepath.Join(resourceDir, replacementFilename)); err != nil {
		t.Fatalf("replacement file was not created: %v", err)
	}
	var cleanupCount int
	if err := db.QueryRow(`SELECT count(*) FROM portal.event_inbox WHERE event_type=$1 AND payload->>'kind'='teaching_resource' AND payload->>'filename'=$2`, fileCleanupEventType, missingFilename).Scan(&cleanupCount); err != nil {
		t.Fatal(err)
	}
	if cleanupCount != 1 {
		t.Fatalf("missing old file cleanup events = %d, want 1", cleanupCount)
	}
	payload, _ := json.Marshal(fileCleanupPayload{Kind: "teaching_resource", Filename: missingFilename})
	if _, err := fileCleanupEventHandler(context.Background(), EventInput{EventID: randomUUID(), EventType: fileCleanupEventType, Payload: payload}); err != nil {
		t.Fatalf("idempotent cleanup of missing file: %v", err)
	}
}

func TestConcurrentResourceFileReplacementsSerializeBeforeWriting(t *testing.T) {
	db := openPortalIntegrationDB(t)
	ownerID, ownerToken := insertSecurityTestUser(t, db, "resource-race-"+strings.ReplaceAll(randomUUID(), "-", "")+"@example.invalid")
	resourceDir := t.TempDir()
	t.Setenv("PERSONAL_PORTAL_RESOURCE_DIR", resourceDir)
	missingSocket := filepath.Join(t.TempDir(), "no-event-core.sock")
	api := &portalAPI{db: db, resourceDir: resourceDir, searchCache: newSearchResultCache(), ipc: IPCClient{SocketPath: missingSocket}}

	oldFilename := randomUUID() + ".pdf"
	if err := writeManagedFile(resourceDir, oldFilename, []byte("%PDF-1.7\noriginal"), 0600); err != nil {
		t.Fatal(err)
	}
	var resourceID string
	if err := db.QueryRow(`INSERT INTO portal.teaching_resources(owner_id,title,description,resource_type,grade,is_public,original_filename,stored_filename,mime_type,file_size) VALUES($1::uuid,'Concurrent resource','','其他',NULL,FALSE,'shared.pdf',$2,'application/pdf',16) RETURNING resource_id::text`, ownerID, oldFilename).Scan(&resourceID); err != nil {
		t.Fatalf("insert concurrent resource: %v", err)
	}
	var cleanupNames []string
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM portal.teaching_resources WHERE resource_id=$1::uuid`, resourceID)
		for _, filename := range append([]string{oldFilename}, cleanupNames...) {
			_, _ = db.Exec(`DELETE FROM portal.event_inbox WHERE event_type=$1 AND payload->>'filename'=$2`, fileCleanupEventType, filename)
			_ = os.Remove(filepath.Join(resourceDir, filename))
		}
	})

	blocker, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = blocker.Rollback() })
	if err := blocker.QueryRow(`SELECT resource_id FROM portal.teaching_resources WHERE resource_id=$1::uuid FOR UPDATE`, resourceID).Scan(new(string)); err != nil {
		t.Fatalf("lock resource row: %v", err)
	}

	const writers = 6
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, writers)
	for i := 0; i < writers; i++ {
		data := []byte(fmt.Sprintf("%%PDF-1.7\nconcurrent revision %02d", i))
		body, err := json.Marshal(map[string]any{
			"title":       fmt.Sprintf("Concurrent revision %02d", i),
			"description": "",
			"type":        "其他",
			"grade":       "",
			"url":         "",
			"filename":    "shared.pdf",
			"data":        base64.StdEncoding.EncodeToString(data),
		})
		if err != nil {
			t.Fatal(err)
		}
		go func(body string) {
			<-start
			results <- securityTestRequest(api, http.MethodPatch, "/resources/"+resourceID, ownerToken, body)
		}(string(body))
	}
	var startedAt time.Time
	if err := db.QueryRow(`SELECT now()`).Scan(&startedAt); err != nil {
		_ = blocker.Rollback()
		t.Fatalf("read database clock before concurrent updates: %v", err)
	}
	close(start)
	deadline := time.Now().Add(5 * time.Second)
	blockedWriters := 0
	for time.Now().Before(deadline) {
		if err := db.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND wait_event_type='Lock' AND state='active'`).Scan(&blockedWriters); err != nil {
			_ = blocker.Rollback()
			t.Fatalf("count concurrent writers waiting on the resource row: %v", err)
		}
		if blockedWriters == writers {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if blockedWriters != writers {
		_ = blocker.Rollback()
		for i := 0; i < writers; i++ {
			<-results
		}
		t.Fatalf("only %d of %d patch requests reached the PostgreSQL row lock", blockedWriters, writers)
	}

	// With a row lock acquired before file creation, all writers must wait here.
	// The old implementation read without FOR UPDATE, wrote every new file, and
	// only then blocked at UPDATE, leaving all but the last file orphaned.
	time.Sleep(100 * time.Millisecond)
	entries, err := os.ReadDir(resourceDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		for _, entry := range entries {
			cleanupNames = append(cleanupNames, entry.Name())
		}
		_ = blocker.Rollback()
		for i := 0; i < writers; i++ {
			<-results
		}
		t.Fatalf("%d files appeared while the resource row was locked; want only the original file", len(entries))
	}

	if err := blocker.Commit(); err != nil {
		t.Fatalf("release resource row lock: %v", err)
	}
	for i := 0; i < writers; i++ {
		response := <-results
		if response.Code != http.StatusOK {
			t.Fatalf("concurrent PATCH returned %d: %s", response.Code, response.Body.String())
		}
	}

	var finalFilename string
	if err := db.QueryRow(`SELECT stored_filename FROM portal.teaching_resources WHERE resource_id=$1::uuid`, resourceID).Scan(&finalFilename); err != nil {
		t.Fatalf("read final stored filename: %v", err)
	}
	rows, err := db.Query(`SELECT payload FROM portal.event_inbox WHERE event_type=$1 AND received_at >= $2 AND payload->>'kind'='teaching_resource'`, fileCleanupEventType, startedAt)
	if err != nil {
		t.Fatalf("read resource cleanup events: %v", err)
	}
	cleanupPayloads := make([][]byte, 0, writers)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		cleanupPayloads = append(cleanupPayloads, payload)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	if len(cleanupPayloads) != writers {
		t.Fatalf("durable cleanup events = %d, want %d (original plus each replaced revision)", len(cleanupPayloads), writers)
	}

	cleanupSet := make(map[string]struct{}, len(cleanupPayloads))
	for _, payload := range cleanupPayloads {
		var cleanup fileCleanupPayload
		if err := json.Unmarshal(payload, &cleanup); err != nil {
			t.Fatal(err)
		}
		if cleanup.Kind != "teaching_resource" || cleanup.Filename == finalFilename {
			t.Fatalf("unexpected cleanup payload: %+v final=%s", cleanup, finalFilename)
		}
		cleanupSet[cleanup.Filename] = struct{}{}
		cleanupNames = append(cleanupNames, cleanup.Filename)
		if _, err := fileCleanupEventHandler(context.Background(), EventInput{EventID: randomUUID(), EventType: fileCleanupEventType, Payload: payload}); err != nil {
			t.Fatalf("process cleanup for %s: %v", cleanup.Filename, err)
		}
	}
	if len(cleanupSet) != writers {
		t.Fatalf("unique cleanup filenames = %d, want %d", len(cleanupSet), writers)
	}

	entries, err = os.ReadDir(resourceDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != finalFilename {
		t.Fatalf("managed directory after cleanup contains %v; want only %s", entries, finalFilename)
	}
	if _, err := os.Stat(filepath.Join(resourceDir, finalFilename)); err != nil {
		t.Fatalf("final referenced file is missing: %v", err)
	}
}
