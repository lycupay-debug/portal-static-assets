package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This integration test exercises the authenticated publish route against real
// PostgreSQL. It verifies the returned representation and durable validation
// event agree with the state committed by the same transaction.
func TestPublishContentReturnsCommittedStateAndPersistsValidationEvent(t *testing.T) {
	db := openPortalIntegrationDB(t)
	ownerID, ownerToken := insertSecurityTestUser(t, db, "publish-"+strings.ReplaceAll(randomUUID(), "-", "")+"@example.invalid")
	slug := "publish-" + strings.ReplaceAll(randomUUID(), "-", "")[:20]

	var contentID string
	if err := db.QueryRow(
		"INSERT INTO portal.portal_content(author_id,title,slug,content_type,tags,summary,body,visibility,status) VALUES($1::uuid,'Publish transaction contract',$2,'article',ARRAY['系统维护']::text[],'summary','publish body','public','draft') RETURNING content_id::text",
		ownerID, slug,
	).Scan(&contentID); err != nil {
		t.Fatalf("insert publish fixture: %v", err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DELETE FROM portal.portal_content WHERE content_id=$1::uuid", contentID); err != nil {
			t.Errorf("cleanup publish content: %v", err)
		}
	})

	api := &portalAPI{
		db:          db,
		ipc:         IPCClient{SocketPath: filepath.Join(t.TempDir(), "missing-event.sock"), Timeout: 100 * time.Millisecond},
		searchCache: newSearchResultCache(),
	}
	response := securityTestRequest(api, http.MethodPost, "/content/"+contentID+"/publish", ownerToken, "")
	if response.Code != http.StatusOK {
		t.Fatalf("publish status = %d body=%s", response.Code, response.Body.String())
	}

	var payload struct {
		Item    map[string]any `json:"item"`
		EventID string         `json:"eventId"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode publish response: %v", err)
	}
	if !portalUUID.MatchString(payload.EventID) {
		t.Fatalf("publish event id = %q, want UUID", payload.EventID)
	}
	if payload.Item["status"] != "published" || payload.Item["publishedAt"] == nil {
		t.Fatalf("publish response item does not contain committed publish state: %#v", payload.Item)
	}

	t.Cleanup(func() {
		if _, err := db.Exec("DELETE FROM portal.event_inbox WHERE event_id=$1::uuid", payload.EventID); err != nil {
			t.Errorf("cleanup publish validation event: %v", err)
		}
	})

	var status string
	var publishedAt sql.NullTime
	if err := db.QueryRow(
		"SELECT status,published_at FROM portal.portal_content WHERE content_id=$1::uuid AND author_id=$2::uuid",
		contentID, ownerID,
	).Scan(&status, &publishedAt); err != nil {
		t.Fatalf("read committed publish state: %v", err)
	}
	if status != "published" || !publishedAt.Valid {
		t.Fatalf("database publish state = %s/%v, want published/non-null", status, publishedAt.Valid)
	}

	var eventType, eventTitle string
	if err := db.QueryRow(
		"SELECT event_type,payload->>'title' FROM portal.event_inbox WHERE event_id=$1::uuid",
		payload.EventID,
	).Scan(&eventType, &eventTitle); err != nil {
		t.Fatalf("read committed validation event: %v", err)
	}
	if eventType != contentValidationEventType || eventTitle != "Publish transaction contract" {
		t.Fatalf("validation event = %s/%q, want %s/title", eventType, eventTitle, contentValidationEventType)
	}
}
