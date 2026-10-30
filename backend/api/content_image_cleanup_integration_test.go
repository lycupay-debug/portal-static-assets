// content_image_cleanup_integration_test.go verifies that permanent deletion preserves managed images still referenced by restorable history.

package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPermanentDeletePreservesImageReferencedByHistoricalVersion(t *testing.T) {
	db := openPortalIntegrationDB(t)
	ownerID, ownerToken := insertSecurityTestUser(t, db, "image-history-"+strings.ReplaceAll(randomUUID(), "-", "")+"@example.invalid")
	uploadDir := t.TempDir()
	api := &portalAPI{db: db, uploadDir: uploadDir, searchCache: newSearchResultCache()}

	imageBytes := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 'v', 'e', 'r', 's', 'i', 'o', 'n', '-', 'h', 'i', 's', 't', 'o', 'r', 'y', '-', 't', 'e', 's', 't'}
	imageName := randomUUID() + ".png"
	if err := writeManagedFile(uploadDir, imageName, imageBytes, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(imageBytes)
	if _, err := db.Exec(`INSERT INTO portal.uploaded_images(filename,uploader_id,mime_type,sha256,file_size_bytes) VALUES($1,$2::uuid,'image/png',$3,$4)`, imageName, ownerID, hex.EncodeToString(sum[:]), len(imageBytes)); err != nil {
		t.Fatalf("insert uploaded image registration: %v", err)
	}

	var targetID, keeperID string
	t.Cleanup(func() {
		if targetID != "" {
			_, _ = db.Exec(`DELETE FROM portal.portal_content WHERE content_id=$1::uuid`, targetID)
		}
		if keeperID != "" {
			_, _ = db.Exec(`DELETE FROM portal.portal_content WHERE content_id=$1::uuid`, keeperID)
		}
		_, _ = db.Exec(`DELETE FROM portal.event_inbox WHERE event_type=$1 AND payload->>'filename'=$2`, fileCleanupEventType, imageName)
		_, _ = db.Exec(`DELETE FROM portal.uploaded_images WHERE filename=$1`, imageName)
		_ = os.Remove(filepath.Join(uploadDir, imageName))
	})
	targetSlug := "deleted-image-" + strings.ReplaceAll(randomUUID(), "-", "")[:16]
	if err := db.QueryRow(`INSERT INTO portal.portal_content(author_id,title,slug,content_type,summary,body,visibility,status,deleted_at) VALUES($1::uuid,'Deleted image owner',$2,'article','','/uploads/' || $3,'private','draft',now()) RETURNING content_id::text`, ownerID, targetSlug, imageName).Scan(&targetID); err != nil {
		t.Fatalf("insert trashed content: %v", err)
	}
	keeperSlug := "image-version-" + strings.ReplaceAll(randomUUID(), "-", "")[:16]
	if err := db.QueryRow(`INSERT INTO portal.portal_content(author_id,title,slug,content_type,summary,body,visibility,status) VALUES($1::uuid,'Version keeper',$2,'article','','current body','private','draft') RETURNING content_id::text`, ownerID, keeperSlug).Scan(&keeperID); err != nil {
		t.Fatalf("insert version keeper content: %v", err)
	}
	var versionID string
	if err := db.QueryRow(`INSERT INTO portal.portal_content_versions(content_id,author_id,version_number,snapshot) VALUES($1::uuid,$2::uuid,1,jsonb_build_object('id',$1::text,'title','Historical image reference','slug',$4::text,'contentType','article','tags',jsonb_build_array('编程'),'parentCategory',NULL::text,'childCategory',NULL::text,'summary','','body',$3::text,'visibility','private','status','draft')) RETURNING version_id::text`, keeperID, ownerID, "/uploads/"+imageName, keeperSlug).Scan(&versionID); err != nil {
		t.Fatalf("insert historical content snapshot: %v", err)
	}

	response := securityTestRequest(api, http.MethodDelete, "/content/"+targetID+"/permanent", ownerToken, "")
	if response.Code != http.StatusOK {
		t.Fatalf("permanent content delete status = %d body=%s", response.Code, response.Body.String())
	}
	var registrationExists bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM portal.uploaded_images WHERE filename=$1)`, imageName).Scan(&registrationExists); err != nil {
		t.Fatal(err)
	}
	if !registrationExists {
		t.Fatal("image registration was removed although another content's restorable version still references it")
	}
	var cleanupEvents int
	if err := db.QueryRow(`SELECT count(*) FROM portal.event_inbox WHERE event_type=$1 AND payload->>'filename'=$2`, fileCleanupEventType, imageName).Scan(&cleanupEvents); err != nil {
		t.Fatal(err)
	}
	if cleanupEvents != 0 {
		t.Fatalf("file cleanup events = %d, want 0 while a historical version references the image", cleanupEvents)
	}
	if _, err := os.Stat(filepath.Join(uploadDir, imageName)); err != nil {
		t.Fatalf("historically referenced image file is missing: %v", err)
	}
	restore := securityTestRequest(api, http.MethodPost, "/content/"+keeperID+"/versions/"+versionID+"/restore", ownerToken, "")
	if restore.Code != http.StatusOK {
		t.Fatalf("restore historical content version status = %d body=%s", restore.Code, restore.Body.String())
	}
	var restoredBody string
	if err := db.QueryRow(`SELECT body FROM portal.portal_content WHERE content_id=$1::uuid`, keeperID).Scan(&restoredBody); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(restoredBody, "/uploads/"+imageName) {
		t.Fatalf("restored content body does not contain historical image reference: %q", restoredBody)
	}
}

func TestConcurrentImageReferenceAndPermanentDeleteSerialize(t *testing.T) {
	db := openPortalIntegrationDB(t)
	ownerID, ownerToken := insertSecurityTestUser(t, db, "image-race-"+strings.ReplaceAll(randomUUID(), "-", "")+"@example.invalid")
	uploadDir := t.TempDir()
	t.Setenv("PERSONAL_PORTAL_UPLOAD_DIR", uploadDir)
	api := &portalAPI{db: db, uploadDir: uploadDir, searchCache: newSearchResultCache()}

	imageBytes := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 'c', 'o', 'n', 'c', 'u', 'r', 'r', 'e', 'n', 't', '-', 'r', 'e', 'f', 'e', 'r', 'e', 'n', 'c', 'e', '-', 't', 'e', 's', 't'}
	imageName := randomUUID() + ".png"
	if err := writeManagedFile(uploadDir, imageName, imageBytes, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(imageBytes)
	if _, err := db.Exec(`INSERT INTO portal.uploaded_images(filename,uploader_id,mime_type,sha256,file_size_bytes) VALUES($1,$2::uuid,'image/png',$3,$4)`, imageName, ownerID, hex.EncodeToString(sum[:]), len(imageBytes)); err != nil {
		t.Fatalf("insert uploaded image registration: %v", err)
	}

	targetSlug := "image-race-target-" + strings.ReplaceAll(randomUUID(), "-", "")[:12]
	createSlug := "image-race-new-" + strings.ReplaceAll(randomUUID(), "-", "")[:12]
	var targetID string
	t.Cleanup(func() {
		if targetID != "" {
			_, _ = db.Exec(`DELETE FROM portal.portal_content WHERE content_id=$1::uuid`, targetID)
		}
		_, _ = db.Exec(`DELETE FROM portal.portal_content WHERE slug=$1`, createSlug)
		_, _ = db.Exec(`DELETE FROM portal.event_inbox WHERE event_type=$1 AND payload->>'filename'=$2`, fileCleanupEventType, imageName)
		_, _ = db.Exec(`DELETE FROM portal.uploaded_images WHERE filename=$1`, imageName)
		_ = os.Remove(filepath.Join(uploadDir, imageName))
	})
	if err := db.QueryRow(`INSERT INTO portal.portal_content(author_id,title,slug,content_type,summary,body,visibility,status,deleted_at) VALUES($1::uuid,'Trashed image reference',$2,'article','','/uploads/' || $3,'private','draft',now()) RETURNING content_id::text`, ownerID, targetSlug, imageName).Scan(&targetID); err != nil {
		t.Fatalf("insert trashed content: %v", err)
	}

	blocker, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if err := lockManagedImageNames(context.Background(), blocker, []string{imageName}); err != nil {
		t.Fatalf("hold image advisory lock: %v", err)
	}

	deleteResults := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		deleteResults <- securityTestRequest(api, http.MethodDelete, "/content/"+targetID+"/permanent", ownerToken, "")
	}()
	waitForImageLockWaiters(t, db, 1)

	requestBody, err := json.Marshal(map[string]any{
		"title":       "Concurrent image reference",
		"slug":        createSlug,
		"contentType": "article",
		"tags":        []string{"编程"},
		"summary":     "",
		"body":        "/uploads/" + imageName,
		"visibility":  "private",
	})
	if err != nil {
		t.Fatal(err)
	}
	createResults := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		createResults <- securityTestRequest(api, http.MethodPost, "/content", ownerToken, string(requestBody))
	}()
	waitForImageLockWaiters(t, db, 2)

	if err := blocker.Commit(); err != nil {
		t.Fatalf("release image advisory lock: %v", err)
	}
	deleteResponse := <-deleteResults
	createResponse := <-createResults
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("permanent delete status = %d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
	if createResponse.Code != http.StatusCreated && createResponse.Code != http.StatusBadRequest {
		t.Fatalf("concurrent content create status = %d body=%s; want 201 or 400 depending on lock order", createResponse.Code, createResponse.Body.String())
	}

	var currentReferences, historicalReferences int
	if err := db.QueryRow(`SELECT count(*) FROM portal.portal_content WHERE position($1 in body)>0`, "/uploads/"+imageName).Scan(&currentReferences); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM portal.portal_content_versions WHERE position($1 in COALESCE(snapshot->>'body',''))>0`, "/uploads/"+imageName).Scan(&historicalReferences); err != nil {
		t.Fatal(err)
	}
	var registered bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM portal.uploaded_images WHERE filename=$1)`, imageName).Scan(&registered); err != nil {
		t.Fatal(err)
	}
	var cleanupCount int
	if err := db.QueryRow(`SELECT count(*) FROM portal.event_inbox WHERE event_type=$1 AND payload->>'filename'=$2`, fileCleanupEventType, imageName).Scan(&cleanupCount); err != nil {
		t.Fatal(err)
	}
	if currentReferences+historicalReferences > 0 {
		if createResponse.Code != http.StatusCreated || !registered || cleanupCount != 0 {
			t.Fatalf("referenced image invariant violated: create=%d registered=%t cleanup_events=%d", createResponse.Code, registered, cleanupCount)
		}
		if _, err := os.Stat(filepath.Join(uploadDir, imageName)); err != nil {
			t.Fatalf("referenced image file missing: %v", err)
		}
		return
	}
	if createResponse.Code != http.StatusBadRequest || registered || cleanupCount != 1 {
		t.Fatalf("unreferenced image invariant violated: create=%d registered=%t cleanup_events=%d", createResponse.Code, registered, cleanupCount)
	}
	var eventID string
	var payload []byte
	if err := db.QueryRow(`SELECT event_id::text,payload FROM portal.event_inbox WHERE event_type=$1 AND payload->>'filename'=$2`, fileCleanupEventType, imageName).Scan(&eventID, &payload); err != nil {
		t.Fatalf("read cleanup event: %v", err)
	}
	if _, err := fileCleanupEventHandler(context.Background(), EventInput{EventID: eventID, EventType: fileCleanupEventType, Payload: payload}); err != nil {
		t.Fatalf("process image cleanup event: %v", err)
	}
	if _, err := os.Stat(filepath.Join(uploadDir, imageName)); !os.IsNotExist(err) {
		t.Fatalf("unreferenced image file still exists or stat failed: %v", err)
	}
}

// waitForImageLockWaiters waits until the expected image-reference transactions are blocked on PostgreSQL locks.
func waitForImageLockWaiters(t *testing.T, db *sql.DB, expected int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND wait_event_type='Lock' AND state='active'`).Scan(&count); err != nil {
			t.Fatalf("count image lock waiters: %v", err)
		}
		if count >= expected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("did not observe %d PostgreSQL lock waiters", expected)
}

func TestConcurrentContentCreateBeforeDeletePreservesImage(t *testing.T) {
	db := openPortalIntegrationDB(t)
	ownerID, ownerToken := insertSecurityTestUser(t, db, "image-create-first-"+strings.ReplaceAll(randomUUID(), "-", "")+"@example.invalid")
	uploadDir := t.TempDir()
	t.Setenv("PERSONAL_PORTAL_UPLOAD_DIR", uploadDir)
	api := &portalAPI{db: db, uploadDir: uploadDir, searchCache: newSearchResultCache()}

	imageBytes := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 'c', 'r', 'e', 'a', 't', 'e', '-', 'f', 'i', 'r', 's', 't', '-', 'i', 'm', 'a', 'g', 'e', '-', 't', 'e', 's', 't'}
	imageName := randomUUID() + ".png"
	if err := writeManagedFile(uploadDir, imageName, imageBytes, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(imageBytes)
	if _, err := db.Exec(`INSERT INTO portal.uploaded_images(filename,uploader_id,mime_type,sha256,file_size_bytes) VALUES($1,$2::uuid,'image/png',$3,$4)`, imageName, ownerID, hex.EncodeToString(sum[:]), len(imageBytes)); err != nil {
		t.Fatalf("insert uploaded image registration: %v", err)
	}

	targetSlug := "image-create-first-target-" + strings.ReplaceAll(randomUUID(), "-", "")[:12]
	createSlug := "image-create-first-new-" + strings.ReplaceAll(randomUUID(), "-", "")[:12]
	var targetID string
	t.Cleanup(func() {
		if targetID != "" {
			_, _ = db.Exec(`DELETE FROM portal.portal_content WHERE content_id=$1::uuid`, targetID)
		}
		_, _ = db.Exec(`DELETE FROM portal.portal_content WHERE slug=$1`, createSlug)
		_, _ = db.Exec(`DELETE FROM portal.event_inbox WHERE event_type=$1 AND payload->>'filename'=$2`, fileCleanupEventType, imageName)
		_, _ = db.Exec(`DELETE FROM portal.uploaded_images WHERE filename=$1`, imageName)
		_ = os.Remove(filepath.Join(uploadDir, imageName))
	})
	if err := db.QueryRow(`INSERT INTO portal.portal_content(author_id,title,slug,content_type,summary,body,visibility,status,deleted_at) VALUES($1::uuid,'Trashed image reference',$2,'article','','/uploads/' || $3,'private','draft',now()) RETURNING content_id::text`, ownerID, targetSlug, imageName).Scan(&targetID); err != nil {
		t.Fatalf("insert trashed content: %v", err)
	}

	blocker, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if err := lockManagedImageNames(context.Background(), blocker, []string{imageName}); err != nil {
		t.Fatalf("hold image advisory lock: %v", err)
	}

	requestBody, err := json.Marshal(map[string]any{
		"title":       "Content created before permanent delete",
		"slug":        createSlug,
		"contentType": "article",
		"tags":        []string{"编程"},
		"summary":     "",
		"body":        "/uploads/" + imageName,
		"visibility":  "private",
	})
	if err != nil {
		t.Fatal(err)
	}
	createResults := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		createResults <- securityTestRequest(api, http.MethodPost, "/content", ownerToken, string(requestBody))
	}()
	waitForImageLockWaiters(t, db, 1)

	deleteResults := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		deleteResults <- securityTestRequest(api, http.MethodDelete, "/content/"+targetID+"/permanent", ownerToken, "")
	}()
	waitForImageLockWaiters(t, db, 2)
	if err := blocker.Commit(); err != nil {
		t.Fatalf("release image advisory lock: %v", err)
	}

	createResponse := <-createResults
	deleteResponse := <-deleteResults
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("content create should win the queued advisory lock: status=%d body=%s", createResponse.Code, createResponse.Body.String())
	}
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("permanent delete status = %d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
	var registered bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM portal.uploaded_images WHERE filename=$1)`, imageName).Scan(&registered); err != nil {
		t.Fatal(err)
	}
	var references int
	if err := db.QueryRow(`SELECT count(*) FROM portal.portal_content WHERE position($1 in body)>0`, "/uploads/"+imageName).Scan(&references); err != nil {
		t.Fatal(err)
	}
	var cleanupEvents int
	if err := db.QueryRow(`SELECT count(*) FROM portal.event_inbox WHERE event_type=$1 AND payload->>'filename'=$2`, fileCleanupEventType, imageName).Scan(&cleanupEvents); err != nil {
		t.Fatal(err)
	}
	if !registered || references != 1 || cleanupEvents != 0 {
		t.Fatalf("create-first invariant violated: registered=%t references=%d cleanup_events=%d", registered, references, cleanupEvents)
	}
	if _, err := os.Stat(filepath.Join(uploadDir, imageName)); err != nil {
		t.Fatalf("image referenced by new content was removed: %v", err)
	}
}
