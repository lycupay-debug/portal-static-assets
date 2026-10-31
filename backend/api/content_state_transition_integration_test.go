package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestContentVersionRestoreReturnsTransactionalState verifies that restoring a
// historical snapshot returns the representation read inside the same
// transaction that persists the restored content and its new history entry.
func TestContentVersionRestoreReturnsTransactionalState(t *testing.T) {
	db := openPortalIntegrationDB(t)
	ownerID, ownerToken := insertSecurityTestUser(t, db, "restore-version-"+strings.ReplaceAll(randomUUID(), "-", "")+"@example.invalid")
	slug := "restore-" + strings.ReplaceAll(randomUUID(), "-", "")[:20]
	var contentID, versionID string
	if err := db.QueryRow(
		`INSERT INTO portal.portal_content(author_id,title,slug,content_type,tags,summary,body,visibility,status,published_at)
		 VALUES($1::uuid,'Current title',$2,'article',ARRAY['系统维护']::text[],'current summary','current body','public','published',now())
		 RETURNING content_id::text`, ownerID, slug,
	).Scan(&contentID); err != nil {
		t.Fatalf("insert content fixture: %v", err)
	}
	snapshot := `{"title":"Restored title","slug":"` + slug + `","contentType":"article","tags":["系统维护"],"parentCategory":"系统","childCategory":"系统维护","summary":"restored summary","body":"restored historical body","visibility":"public","status":"published"}`
	if err := db.QueryRow(
		`INSERT INTO portal.portal_content_versions(content_id,author_id,version_number,snapshot)
		 VALUES($1::uuid,$2::uuid,1,$3::jsonb) RETURNING version_id::text`, contentID, ownerID, snapshot,
	).Scan(&versionID); err != nil {
		t.Fatalf("insert historical version fixture: %v", err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM portal.portal_content WHERE content_id=$1::uuid`, contentID); err != nil {
			t.Errorf("cleanup restored content: %v", err)
		}
	})

	api := &portalAPI{db: db, searchCache: newSearchResultCache()}
	response := securityTestRequest(api, http.MethodPost, "/content/"+contentID+"/versions/"+versionID+"/restore", ownerToken, "")
	if response.Code != http.StatusOK {
		t.Fatalf("restore status = %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Item    map[string]any `json:"item"`
		EventID string         `json:"eventId"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode restore response: %v", err)
	}
	if payload.Item["title"] != "Restored title" || payload.Item["body"] != "restored historical body" || payload.Item["status"] != "published" {
		t.Fatalf("restore response item = %#v", payload.Item)
	}
	var title, body, status string
	if err := db.QueryRow(`SELECT title,body,status FROM portal.portal_content WHERE content_id=$1::uuid`, contentID).Scan(&title, &body, &status); err != nil {
		t.Fatalf("read committed restored content: %v", err)
	}
	if title != payload.Item["title"] || body != payload.Item["body"] || status != payload.Item["status"] {
		t.Fatalf("committed content = %q/%q/%q, response = %#v", title, body, status, payload.Item)
	}
	var versions int
	if err := db.QueryRow(`SELECT count(*) FROM portal.portal_content_versions WHERE content_id=$1::uuid`, contentID).Scan(&versions); err != nil {
		t.Fatalf("count content versions: %v", err)
	}
	if versions != 2 {
		t.Fatalf("content version count = %d, want original plus pre-restore snapshot", versions)
	}
	if !portalUUID.MatchString(payload.EventID) {
		t.Fatalf("published restore eventId = %q, want UUID", payload.EventID)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM portal.event_inbox WHERE event_id=$1::uuid`, payload.EventID); err != nil {
			t.Errorf("cleanup restore validation event: %v", err)
		}
	})
	var eventType, eventTitle string
	if err := db.QueryRow(`SELECT event_type,payload->>'title' FROM portal.event_inbox WHERE event_id=$1::uuid`, payload.EventID).Scan(&eventType, &eventTitle); err != nil {
		t.Fatalf("read restore validation event: %v", err)
	}
	if eventType != contentValidationEventType || eventTitle != "Restored title" {
		t.Fatalf("restore validation event = %s/%q", eventType, eventTitle)
	}
}

// TestUnpublishReturnsTransactionalState verifies that the unpublish response
// is selected before commit and agrees with the persisted owner-scoped state.
func TestUnpublishReturnsTransactionalState(t *testing.T) {
	db := openPortalIntegrationDB(t)
	ownerID, ownerToken := insertSecurityTestUser(t, db, "unpublish-"+strings.ReplaceAll(randomUUID(), "-", "")+"@example.invalid")
	_, otherToken := insertSecurityTestUser(t, db, "unpublish-other-"+strings.ReplaceAll(randomUUID(), "-", "")+"@example.invalid")
	slug := "unpublish-" + strings.ReplaceAll(randomUUID(), "-", "")[:20]
	var contentID string
	if err := db.QueryRow(
		`INSERT INTO portal.portal_content(author_id,title,slug,content_type,tags,summary,body,visibility,status,published_at)
		 VALUES($1::uuid,'Unpublish transaction contract',$2,'article',ARRAY['系统维护']::text[],'summary','published body','public','published',now())
		 RETURNING content_id::text`, ownerID, slug,
	).Scan(&contentID); err != nil {
		t.Fatalf("insert published content fixture: %v", err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM portal.portal_content WHERE content_id=$1::uuid`, contentID); err != nil {
			t.Errorf("cleanup unpublished content: %v", err)
		}
	})

	api := &portalAPI{db: db, searchCache: newSearchResultCache()}
	if denied := securityTestRequest(api, http.MethodPost, "/content/"+contentID+"/unpublish", otherToken, ""); denied.Code != http.StatusNotFound {
		t.Fatalf("non-owner unpublish status = %d body=%s, want 404", denied.Code, denied.Body.String())
	}
	response := securityTestRequest(api, http.MethodPost, "/content/"+contentID+"/unpublish", ownerToken, "")
	if response.Code != http.StatusOK {
		t.Fatalf("unpublish status = %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Item map[string]any `json:"item"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode unpublish response: %v", err)
	}
	if payload.Item["status"] != "unpublished" {
		t.Fatalf("unpublish response status = %v, want unpublished", payload.Item["status"])
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM portal.portal_content WHERE content_id=$1::uuid AND author_id=$2::uuid`, contentID, ownerID).Scan(&status); err != nil {
		t.Fatalf("read committed unpublish state: %v", err)
	}
	if status != payload.Item["status"] {
		t.Fatalf("committed status = %q, response status = %v", status, payload.Item["status"])
	}
}
