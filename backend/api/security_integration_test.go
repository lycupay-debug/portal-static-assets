// security_integration_test.go verifies PostgreSQL-backed RBAC and object ownership boundaries.

package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These PostgreSQL integration tests verify real session, role, and owner-scoped
// file/content paths using the same least-privilege database role as the API.
func insertSecurityTestUser(t *testing.T, db *sql.DB, email string) (userID, token string) {
	t.Helper()
	if err := db.QueryRow(`INSERT INTO portal.portal_users(email,display_name,password_hash) VALUES($1,'Security integration','unused') RETURNING user_id::text`, email).Scan(&userID); err != nil {
		t.Fatalf("insert security test user %s: %v", email, err)
	}
	token = randomUUID()
	if _, err := db.Exec(`INSERT INTO portal.portal_auth_sessions(user_id,token_hash,expires_at) VALUES($1::uuid,$2,now()+interval '1 hour')`, userID, tokenHash(token)); err != nil {
		t.Fatalf("insert security test session: %v", err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM portal.portal_users WHERE user_id=$1::uuid`, userID); err != nil {
			t.Errorf("delete security test user %s: %v", email, err)
		}
	})
	return userID, token
}

func securityTestRequest(api *portalAPI, method, path, token string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "https://portal.example"+path, strings.NewReader(body))
	if token != "" {
		request.AddCookie(&http.Cookie{Name: portalCookie, Value: token})
	}
	recorder := httptest.NewRecorder()
	api.ServeHTTP(recorder, request)
	return recorder
}

func TestPortalObjectOwnershipAndUploadAccess(t *testing.T) {
	db := openPortalIntegrationDB(t)
	ownerID, ownerToken := insertSecurityTestUser(t, db, "owner-"+strings.ReplaceAll(randomUUID(), "-", "")+"@example.invalid")
	otherID, otherToken := insertSecurityTestUser(t, db, "other-"+strings.ReplaceAll(randomUUID(), "-", "")+"@example.invalid")
	_ = otherID

	uploadDir := t.TempDir()
	resourceDir := t.TempDir()
	api := &portalAPI{db: db, uploadDir: uploadDir, resourceDir: resourceDir, searchCache: newSearchResultCache()}

	storedFilename := randomUUID() + ".pdf"
	resourceBytes := []byte("%PDF-1.7\nowner-only resource")
	if err := writeManagedFile(resourceDir, storedFilename, resourceBytes, 0600); err != nil {
		t.Fatal(err)
	}
	var resourceID string
	if err := db.QueryRow(`INSERT INTO portal.teaching_resources(owner_id,title,description,resource_type,grade,is_public,original_filename,stored_filename,mime_type,file_size) VALUES($1::uuid,'Owner resource','','其他',NULL,FALSE,'owner.pdf',$2,'application/pdf',$3) RETURNING resource_id::text`, ownerID, storedFilename, len(resourceBytes)).Scan(&resourceID); err != nil {
		t.Fatalf("insert owner resource: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM portal.teaching_resources WHERE resource_id=$1::uuid`, resourceID)
		_ = os.Remove(filepath.Join(resourceDir, storedFilename))
	})

	privateSlug := "private-" + strings.ReplaceAll(randomUUID(), "-", "")[:16]
	var contentID string
	if err := db.QueryRow(`INSERT INTO portal.portal_content(author_id,title,slug,content_type,summary,body,visibility,status) VALUES($1::uuid,'Owner content',$2,'article','','private body','private','draft') RETURNING content_id::text`, ownerID, privateSlug).Scan(&contentID); err != nil {
		t.Fatalf("insert owner content: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM portal.portal_content WHERE content_id=$1::uuid`, contentID) })

	if got := securityTestRequest(api, http.MethodGet, "/content/"+contentID, ownerToken, "").Code; got != http.StatusOK {
		t.Fatalf("owner content read status = %d, want 200", got)
	}
	if got := securityTestRequest(api, http.MethodGet, "/content/"+contentID, otherToken, "").Code; got != http.StatusNotFound {
		t.Fatalf("non-owner content read status = %d, want 404", got)
	}
	if got := securityTestRequest(api, http.MethodGet, "/resources/"+resourceID+"/download", ownerToken, "").Code; got != http.StatusOK {
		t.Fatalf("owner resource download status = %d, want 200", got)
	}
	if got := securityTestRequest(api, http.MethodGet, "/resources/"+resourceID+"/download", otherToken, "").Code; got != http.StatusNotFound {
		t.Fatalf("non-owner resource download status = %d, want 404", got)
	}

	imageBytes := []byte("\x89PNG\r\n\x1a\nminimal-test-png")
	requestBody, err := json.Marshal(map[string]string{
		"mime": "image/png",
		"data": base64.StdEncoding.EncodeToString(imageBytes),
	})
	if err != nil {
		t.Fatal(err)
	}
	upload := securityTestRequest(api, http.MethodPost, "/uploads", ownerToken, string(requestBody))
	if upload.Code != http.StatusCreated {
		t.Fatalf("owner image upload status = %d body=%s", upload.Code, upload.Body.String())
	}
	var uploadResult struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(upload.Body.Bytes(), &uploadResult); err != nil || !strings.HasPrefix(uploadResult.URL, "/api/uploads/") {
		t.Fatalf("invalid upload response: %s err=%v", upload.Body.String(), err)
	}
	imagePath := strings.TrimPrefix(uploadResult.URL, "/api")
	imageName := strings.TrimPrefix(imagePath, "/uploads/")
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM portal.uploaded_images WHERE filename=$1`, imageName)
		_ = os.Remove(filepath.Join(uploadDir, imageName))
	})
	if got := securityTestRequest(api, http.MethodGet, imagePath, ownerToken, "").Code; got != http.StatusOK {
		t.Fatalf("image owner read status = %d, want 200", got)
	}
	if got := securityTestRequest(api, http.MethodGet, imagePath, otherToken, "").Code; got != http.StatusNotFound {
		t.Fatalf("non-owner private image read status = %d, want 404", got)
	}
	if got := securityTestRequest(api, http.MethodGet, imagePath, "", "").Code; got != http.StatusNotFound {
		t.Fatalf("anonymous private image read status = %d, want 404", got)
	}

	publicSlug := "public-" + strings.ReplaceAll(randomUUID(), "-", "")[:16]
	var publicContentID string
	if err := db.QueryRow(`INSERT INTO portal.portal_content(author_id,title,slug,content_type,summary,body,visibility,status,published_at) VALUES($1::uuid,'Public image reference',$2,'article','','/uploads/' || $3,'public','published',now()) RETURNING content_id::text`, otherID, publicSlug, imageName).Scan(&publicContentID); err != nil {
		t.Fatalf("insert public image reference: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM portal.portal_content WHERE content_id=$1::uuid`, publicContentID) })
	if got := securityTestRequest(api, http.MethodGet, imagePath, "", "").Code; got != http.StatusOK {
		t.Fatalf("publicly referenced image status = %d, want 200", got)
	}
}

func TestPortalRBACPermissionMatrix(t *testing.T) {
	db := openPortalIntegrationDB(t)
	api := &portalAPI{db: db, searchCache: newSearchResultCache()}
	regularEmail := "regular-" + strings.ReplaceAll(randomUUID(), "-", "") + "@example.invalid"
	regularID, regularToken := insertSecurityTestUser(t, db, regularEmail)
	if got := securityTestRequest(api, http.MethodGet, "/admin/users", regularToken, "").Code; got != http.StatusForbidden {
		t.Fatalf("ordinary user RBAC read status = %d, want 403", got)
	}

	roleKey := "compliance_read_" + strings.ReplaceAll(randomUUID(), "-", "")[:12]
	if _, err := db.Exec(`INSERT INTO portal.roles(role_key,display_name,description,is_system) VALUES($1,'Read-only test role','integration test',FALSE)`, roleKey); err != nil {
		t.Fatalf("insert read-only role: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM portal.portal_user_roles WHERE role_key=$1`, roleKey)
		_, _ = db.Exec(`DELETE FROM portal.role_permissions WHERE role_key=$1`, roleKey)
		_, _ = db.Exec(`DELETE FROM portal.roles WHERE role_key=$1`, roleKey)
	})
	if _, err := db.Exec(`INSERT INTO portal.role_permissions(role_key,permission_key) VALUES($1,'portal.compliance.read')`, roleKey); err != nil {
		t.Fatalf("assign read-only permission: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO portal.portal_user_roles(user_id,role_key,assigned_by) VALUES($1::uuid,$2,'integration-test')`, regularID, roleKey); err != nil {
		t.Fatalf("assign read-only role: %v", err)
	}
	if got := securityTestRequest(api, http.MethodGet, "/compliance/overview", regularToken, "").Code; got != http.StatusOK {
		t.Fatalf("compliance read permission status = %d, want 200", got)
	}
	if got := securityTestRequest(api, http.MethodPost, "/compliance/assets", regularToken, `{}`).Code; got != http.StatusForbidden {
		t.Fatalf("read-only role write status = %d, want 403", got)
	}
	// The compatibility is_admin column must not bypass the RBAC permission
	// matrix when an account has only portal.compliance.read.
	if _, err := db.Exec(`UPDATE portal.portal_users SET is_admin=TRUE WHERE user_id=$1::uuid`, regularID); err != nil {
		t.Fatalf("set legacy admin compatibility flag: %v", err)
	}
	if got := securityTestRequest(api, http.MethodPost, "/compliance/assets", regularToken, `{}`).Code; got != http.StatusForbidden {
		t.Fatalf("legacy is_admin flag bypassed compliance RBAC: status=%d, want 403", got)
	}
	if _, err := db.Exec(`UPDATE portal.portal_users SET is_admin=FALSE WHERE user_id=$1::uuid`, regularID); err != nil {
		t.Fatalf("clear legacy admin compatibility flag: %v", err)
	}

	var existingAdmin int
	if err := db.QueryRow(`SELECT count(*) FROM portal.portal_users WHERE is_admin=TRUE`).Scan(&existingAdmin); err != nil {
		t.Fatal(err)
	}
	if existingAdmin != 0 {
		t.Skip("integration database already contains an admin account; designated-admin bootstrap case is skipped")
	}
	superID, superToken := insertSecurityTestUser(t, db, "lycupay@gmail.com")
	if _, err := db.Exec(`UPDATE portal.portal_users SET is_admin=TRUE WHERE user_id=$1::uuid`, superID); err != nil {
		t.Fatalf("set designated admin flag: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO portal.portal_user_roles(user_id,role_key,assigned_by) VALUES($1::uuid,'super_admin','integration-test')`, superID); err != nil {
		t.Fatalf("assign super-admin role: %v", err)
	}
	if got := securityTestRequest(api, http.MethodGet, "/admin/users", superToken, "").Code; got != http.StatusOK {
		t.Fatalf("designated super-admin RBAC read status = %d, want 200", got)
	}
	if got := securityTestRequest(api, http.MethodGet, "/admin/users", regularToken, "").Code; got != http.StatusForbidden {
		t.Fatalf("ordinary user RBAC read after admin bootstrap = %d, want 403", got)
	}
}
