package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestContentLifecyclePreservesOwnerScope verifies author-scoped reads, trash,
// restore, and permanent deletion through the content route dispatcher.
func TestContentLifecyclePreservesOwnerScope(t *testing.T) {
	db := openPortalIntegrationDB(t)
	ownerID, ownerToken := insertSecurityTestUser(t, db, "content-owner-"+strings.ReplaceAll(randomUUID(), "-", "")+"@example.invalid")
	otherID, _ := insertSecurityTestUser(t, db, "content-other-"+strings.ReplaceAll(randomUUID(), "-", "")+"@example.invalid")

	insertContent := func(authorID, title string) string {
		t.Helper()
		slug := "phase3-content-" + strings.ReplaceAll(randomUUID(), "-", "")[:20]
		var id string
		err := db.QueryRow(
			`INSERT INTO portal.portal_content(author_id,title,slug,content_type,summary,body,visibility,status)
			 VALUES($1::uuid,$2,$3,'article','','lifecycle body','private','draft') RETURNING content_id::text`,
			authorID, title, slug,
		).Scan(&id)
		if err != nil {
			t.Fatalf("insert content %q: %v", title, err)
		}
		t.Cleanup(func() {
			if _, err := db.Exec("DELETE FROM portal.portal_content WHERE content_id=$1::uuid", id); err != nil {
				t.Errorf("cleanup content %s: %v", id, err)
			}
		})
		return id
	}

	ownedID := insertContent(ownerID, "Owned content lifecycle")
	otherContentID := insertContent(otherID, "Other author's content")
	api := &portalAPI{db: db, searchCache: newSearchResultCache()}

	list := securityTestRequest(api, http.MethodGet, "/content?q=Owned%20content%20lifecycle", ownerToken, "")
	if list.Code != http.StatusOK {
		t.Fatalf("owner content list status = %d body=%s", list.Code, list.Body.String())
	}
	var listPayload struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode content list: %v", err)
	}
	if !containsContentID(listPayload.Items, ownedID) || containsContentID(listPayload.Items, otherContentID) {
		t.Fatalf("owner content list contains unexpected IDs: %+v", listPayload.Items)
	}

	if got := securityTestRequest(api, http.MethodGet, "/content/"+otherContentID, ownerToken, "").Code; got != http.StatusNotFound {
		t.Fatalf("non-owner content read status = %d, want 404", got)
	}
	if got := securityTestRequest(api, http.MethodDelete, "/content/"+otherContentID, ownerToken, "").Code; got != http.StatusNotFound {
		t.Fatalf("non-owner content trash status = %d, want 404", got)
	}
	if got := securityTestRequest(api, http.MethodDelete, "/content/"+ownedID, ownerToken, "").Code; got != http.StatusOK {
		t.Fatalf("owner content trash status = %d, want 200", got)
	}

	trash := securityTestRequest(api, http.MethodGet, "/content/trash?q=Owned%20content%20lifecycle", ownerToken, "")
	if trash.Code != http.StatusOK {
		t.Fatalf("owner content trash list status = %d body=%s", trash.Code, trash.Body.String())
	}
	var trashPayload struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(trash.Body.Bytes(), &trashPayload); err != nil {
		t.Fatalf("decode content trash list: %v", err)
	}
	if !containsContentID(trashPayload.Items, ownedID) {
		t.Fatalf("owner trash list does not contain %s: %+v", ownedID, trashPayload.Items)
	}

	restore := securityTestRequest(api, http.MethodPost, "/content/"+ownedID+"/restore", ownerToken, "")
	if restore.Code != http.StatusOK {
		t.Fatalf("owner content restore status = %d body=%s", restore.Code, restore.Body.String())
	}
	var deletedAt sql.NullTime
	if err := db.QueryRow("SELECT deleted_at FROM portal.portal_content WHERE content_id=$1::uuid", ownedID).Scan(&deletedAt); err != nil {
		t.Fatalf("read restored content: %v", err)
	}
	if deletedAt.Valid {
		t.Fatal("restored content still has deleted_at")
	}

	if got := securityTestRequest(api, http.MethodDelete, "/content/"+ownedID, ownerToken, "").Code; got != http.StatusOK {
		t.Fatalf("second content trash status = %d, want 200", got)
	}
	permanent := securityTestRequest(api, http.MethodDelete, "/content/"+ownedID+"/permanent", ownerToken, "")
	if permanent.Code != http.StatusOK {
		t.Fatalf("content permanent delete status = %d body=%s", permanent.Code, permanent.Body.String())
	}
	var remaining int
	if err := db.QueryRow("SELECT count(*) FROM portal.portal_content WHERE content_id=$1::uuid", ownedID).Scan(&remaining); err != nil {
		t.Fatalf("verify permanent content deletion: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("permanently deleted content row count = %d, want 0", remaining)
	}
}

func containsContentID(items []struct {
	ID string `json:"id"`
}, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
