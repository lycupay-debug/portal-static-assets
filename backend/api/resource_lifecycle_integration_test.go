package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestResourceLifecyclePreservesOwnerScope verifies collection reads, trash,
// restore, and permanent deletion through the public HTTP route dispatcher.
func TestResourceLifecyclePreservesOwnerScope(t *testing.T) {
	db := openPortalIntegrationDB(t)
	ownerID, ownerToken := insertSecurityTestUser(t, db, "resource-owner-"+strings.ReplaceAll(randomUUID(), "-", "")+"@example.invalid")
	otherID, _ := insertSecurityTestUser(t, db, "resource-other-"+strings.ReplaceAll(randomUUID(), "-", "")+"@example.invalid")

	insertResource := func(ownerID, title string) string {
		t.Helper()
		var id string
		err := db.QueryRow(
			`INSERT INTO portal.teaching_resources(owner_id,title,description,resource_type,grade,is_public,resource_url)
			 VALUES($1::uuid,$2,'','其他',NULL,FALSE,'https://example.invalid/lifecycle') RETURNING resource_id::text`,
			ownerID, title,
		).Scan(&id)
		if err != nil {
			t.Fatalf("insert resource %q: %v", title, err)
		}
		t.Cleanup(func() {
			if _, err := db.Exec("DELETE FROM portal.teaching_resources WHERE resource_id=$1::uuid", id); err != nil {
				t.Errorf("cleanup resource %s: %v", id, err)
			}
		})
		return id
	}

	ownedID := insertResource(ownerID, "Owned lifecycle resource")
	otherResourceID := insertResource(otherID, "Other owner's resource")
	api := &portalAPI{db: db, resourceDir: t.TempDir(), searchCache: newSearchResultCache()}

	list := securityTestRequest(api, http.MethodGet, "/resources?q=Owned%20lifecycle%20resource", ownerToken, "")
	if list.Code != http.StatusOK {
		t.Fatalf("owner resource list status = %d body=%s", list.Code, list.Body.String())
	}
	var listPayload struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode resource list: %v", err)
	}
	if len(listPayload.Items) != 1 || listPayload.Items[0].ID != ownedID {
		t.Fatalf("owner resource list = %+v, want only %s", listPayload.Items, ownedID)
	}

	if got := securityTestRequest(api, http.MethodDelete, "/resources/"+otherResourceID, ownerToken, "").Code; got != http.StatusNotFound {
		t.Fatalf("non-owner trash status = %d, want 404", got)
	}
	if got := securityTestRequest(api, http.MethodDelete, "/resources/"+ownedID, ownerToken, "").Code; got != http.StatusOK {
		t.Fatalf("owner trash status = %d, want 200", got)
	}

	trash := securityTestRequest(api, http.MethodGet, "/resources/trash?q=Owned%20lifecycle%20resource", ownerToken, "")
	if trash.Code != http.StatusOK {
		t.Fatalf("owner trash list status = %d body=%s", trash.Code, trash.Body.String())
	}
	var trashPayload struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(trash.Body.Bytes(), &trashPayload); err != nil {
		t.Fatalf("decode resource trash list: %v", err)
	}
	if len(trashPayload.Items) != 1 || trashPayload.Items[0].ID != ownedID {
		t.Fatalf("owner trash list = %+v, want only %s", trashPayload.Items, ownedID)
	}

	restore := securityTestRequest(api, http.MethodPost, "/resources/"+ownedID+"/restore", ownerToken, "")
	if restore.Code != http.StatusOK {
		t.Fatalf("owner restore status = %d body=%s", restore.Code, restore.Body.String())
	}
	var deletedAt sql.NullTime
	if err := db.QueryRow("SELECT deleted_at FROM portal.teaching_resources WHERE resource_id=$1::uuid", ownedID).Scan(&deletedAt); err != nil {
		t.Fatalf("read restored resource: %v", err)
	}
	if deletedAt.Valid {
		t.Fatal("restored resource still has deleted_at")
	}

	if got := securityTestRequest(api, http.MethodDelete, "/resources/"+ownedID, ownerToken, "").Code; got != http.StatusOK {
		t.Fatalf("second owner trash status = %d, want 200", got)
	}
	permanent := securityTestRequest(api, http.MethodDelete, "/resources/"+ownedID+"/permanent", ownerToken, "")
	if permanent.Code != http.StatusOK {
		t.Fatalf("owner permanent delete status = %d body=%s", permanent.Code, permanent.Body.String())
	}
	var remaining int
	if err := db.QueryRow("SELECT count(*) FROM portal.teaching_resources WHERE resource_id=$1::uuid", ownedID).Scan(&remaining); err != nil {
		t.Fatalf("verify permanent delete: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("permanently deleted resource row count = %d, want 0", remaining)
	}

	if got := securityTestRequest(api, http.MethodPost, "/resources/"+ownedID+"/restore", ownerToken, "").Code; got != http.StatusNotFound {
		t.Fatalf("restore after permanent deletion status = %d, want %d", got, http.StatusNotFound)
	}
}
