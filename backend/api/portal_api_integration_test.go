package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
)

// Opt-in against a dedicated PostgreSQL database. It exercises the same Go
// handler used in production and cleans up every row created by this test.
func TestPortalAPIEndToEnd(t *testing.T) {
	urlFile := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_TEST_DATABASE_URL_FILE"))
	if urlFile == "" {
		t.Skip("set PERSONAL_PORTAL_TEST_DATABASE_URL_FILE to enable portal API integration tests")
	}
	data, err := os.ReadFile(urlFile)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(8)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	password := "portal-api-test-password-2026"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 4)
	if err != nil {
		t.Fatal(err)
	}
	email := "go-api-test-" + randomUUID()[:8] + "@example.invalid"
	var userID string
	err = db.QueryRowContext(ctx, `INSERT INTO portal.portal_users(email,display_name,password_hash) VALUES($1,'Go API Integration', $2) RETURNING user_id::text`, email, string(hash)).Scan(&userID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanCancel()
		_, _ = db.ExecContext(cleanCtx, `DELETE FROM portal.portal_content_interactions WHERE user_id=$1`, userID)
		_, _ = db.ExecContext(cleanCtx, `DELETE FROM portal.portal_content_versions WHERE author_id=$1`, userID)
		_, _ = db.ExecContext(cleanCtx, `DELETE FROM portal.uploaded_images WHERE uploader_id=$1`, userID)
		_, _ = db.ExecContext(cleanCtx, `DELETE FROM portal.portal_content WHERE author_id=$1`, userID)
		_, _ = db.ExecContext(cleanCtx, `DELETE FROM portal.teaching_resources WHERE owner_id=$1`, userID)
		_, _ = db.ExecContext(cleanCtx, `DELETE FROM portal.portal_auth_sessions WHERE user_id=$1`, userID)
		_, _ = db.ExecContext(cleanCtx, `DELETE FROM portal.portal_users WHERE user_id=$1`, userID)
	}()

	api := newPortalAPI(db, IPCClient{SocketPath: t.TempDir() + "/not-running.sock", Timeout: 100 * time.Millisecond})
	api.uploadDir = t.TempDir()
	api.resourceDir = t.TempDir()
	metrics := httptest.NewRecorder()
	eventMetricsHandler(db).ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metrics.Code != http.StatusOK || !strings.Contains(metrics.Body.String(), `personal_portal_events{status="received"}`) {
		t.Fatalf("event metrics contract mismatch: status=%d body=%s", metrics.Code, metrics.Body.String())
	}
	login := requestJSON(t, api, http.MethodPost, "/auth/login", map[string]any{"email": email, "password": password}, nil, http.StatusOK)
	cookie := ""
	for _, c := range login.Result().Cookies() {
		if c.Name == portalCookie {
			cookie = c.Value
		}
	}
	if cookie == "" {
		t.Fatal("login did not set the session cookie")
	}
	sessionCookie := &http.Cookie{Name: portalCookie, Value: cookie}

	// Public policy routes expose only the non-copyright policies retained outside
	// the internal compliance center. Public copyright complaint intake is removed.
	requestJSON(t, api, http.MethodGet, "/public/legal", nil, nil, http.StatusOK)
	requestJSON(t, api, http.MethodGet, "/public/legal/copyright-notice", nil, nil, http.StatusNotFound)
	requestJSON(t, api, http.MethodPost, "/public/complaints", map[string]any{"name": "Rights Holder", "email": "rights@example.invalid", "workDescription": "Example protected work", "infringingUrls": []string{"https://example.invalid/work"}, "statementTruthful": true}, nil, http.StatusNotFound)
	requestJSON(t, api, http.MethodGet, "/compliance/overview", nil, sessionCookie, http.StatusForbidden)

	if _, err := db.ExecContext(ctx, `UPDATE portal.portal_users SET is_admin=TRUE WHERE user_id=$1`, userID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/compliance/overview", "/compliance/assets?limit=5", "/compliance/licenses", "/compliance/documents", "/compliance/scans", "/compliance/audit"} {
		requestJSON(t, api, http.MethodGet, path, nil, sessionCookie, http.StatusOK)
	}
	var testLicenseID string
	if err := db.QueryRowContext(ctx, "INSERT INTO portal.compliance_license_catalog(spdx_id,license_name,license_version,license_family,canonical_url,full_text,commercial_use_allowed,modification_allowed,redistribution_allowed,attribution_required,source_code_offer_required,review_status) VALUES('MIT','MIT','', 'permissive','https://spdx.org/licenses/MIT.html','MIT test license text',TRUE,TRUE,TRUE,TRUE,FALSE,'unreviewed') RETURNING license_id::text").Scan(&testLicenseID); err != nil {
		t.Fatal(err)
	}
	licenseDetail := requestJSON(t, api, http.MethodGet, "/compliance/licenses/"+testLicenseID, nil, sessionCookie, http.StatusOK)
	if !bytes.Contains(licenseDetail.Body.Bytes(), []byte("MIT test license text")) {
		t.Fatalf("license detail did not return the captured license text: %s", licenseDetail.Body.String())
	}
	requestJSON(t, api, http.MethodPatch, "/compliance/documents/copyright-notice", map[string]any{"status": "published", "approvedBy": "integration-test", "legalReviewAt": "2026-10-17T00:00:00Z", "missingInformation": []string{}}, sessionCookie, http.StatusConflict)
	if _, err := db.ExecContext(ctx, `UPDATE portal.portal_users SET is_admin=FALSE WHERE user_id=$1`, userID); err != nil {
		t.Fatal(err)
	}

	me := requestJSON(t, api, http.MethodGet, "/auth/me", nil, &http.Cookie{Name: portalCookie, Value: cookie}, http.StatusOK)
	if me.Body == nil || !bytes.Contains(me.Body.Bytes(), []byte(email)) {
		t.Fatal("/auth/me did not return the authenticated user")
	}

	slug := "go-api-test-" + randomUUID()[:8]
	created := requestJSON(t, api, http.MethodPost, "/content", map[string]any{"title": "Go API integration", "slug": slug, "contentType": "article", "tags": []string{"教育"}, "parentCategory": "年级", "childCategory": "初一", "summary": "integration", "body": "Go is the only business API", "visibility": "private"}, sessionCookie, http.StatusCreated)
	var contentEnvelope struct {
		Item map[string]any `json:"item"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &contentEnvelope); err != nil {
		t.Fatal(err)
	}
	contentID, _ := contentEnvelope.Item["id"].(string)
	if contentID == "" || contentEnvelope.Item["parentCategory"] != "年级" || contentEnvelope.Item["childCategory"] != "初一" {
		t.Fatalf("create response contract mismatch: %#v", contentEnvelope.Item)
	}

	invalid := requestJSON(t, api, http.MethodPatch, "/content/"+contentID, map[string]any{"parentCategory": "年级", "childCategory": "国考"}, sessionCookie, http.StatusBadRequest)
	if !bytes.Contains(invalid.Body.Bytes(), []byte("子类必须属于当前选择的父类")) {
		t.Fatalf("invalid taxonomy error response: %s", invalid.Body.String())
	}
	cleared := requestJSON(t, api, http.MethodPatch, "/content/"+contentID, map[string]any{"parentCategory": nil, "childCategory": nil}, sessionCookie, http.StatusOK)
	var clearedEnvelope struct {
		Item map[string]any `json:"item"`
	}
	_ = json.Unmarshal(cleared.Body.Bytes(), &clearedEnvelope)
	if clearedEnvelope.Item["parentCategory"] != nil || clearedEnvelope.Item["childCategory"] != nil {
		t.Fatalf("explicit null taxonomy fields were not cleared: %#v", clearedEnvelope.Item)
	}
	versions := requestJSON(t, api, http.MethodGet, "/content/"+contentID+"/versions", nil, sessionCookie, http.StatusOK)
	if !bytes.Contains(versions.Body.Bytes(), []byte("versionNumber")) {
		t.Fatalf("version history contract mismatch: %s", versions.Body.String())
	}

	// User content publication is no longer coupled to the independent compliance center.
	requestJSON(t, api, http.MethodPatch, "/content/"+contentID, map[string]any{"visibility": "public"}, sessionCookie, http.StatusOK)
	requestJSON(t, api, http.MethodPost, "/content/"+contentID+"/publish", map[string]any{}, sessionCookie, http.StatusOK)
	requestJSON(t, api, http.MethodPost, "/content/"+contentID+"/unpublish", map[string]any{}, sessionCookie, http.StatusOK)
	requestJSON(t, api, http.MethodPatch, "/content/"+contentID, map[string]any{"body": "A changed revision after publication."}, sessionCookie, http.StatusOK)
	requestJSON(t, api, http.MethodPost, "/content/"+contentID+"/publish", map[string]any{}, sessionCookie, http.StatusOK)

	resource := requestJSON(t, api, http.MethodPost, "/resources", map[string]any{"title": "URL-only resource", "description": "integration", "type": "电子书", "url": "https://example.com/material"}, sessionCookie, http.StatusCreated)
	var resourceEnvelope struct {
		Item map[string]any `json:"item"`
	}
	_ = json.Unmarshal(resource.Body.Bytes(), &resourceEnvelope)
	resourceID, _ := resourceEnvelope.Item["id"].(string)
	if resourceID == "" || resourceEnvelope.Item["url"] != "https://example.com/material" || resourceEnvelope.Item["type"] != "电子书" || resourceEnvelope.Item["isPublic"] != false {
		t.Fatalf("new resources should be private by default; response: %#v", resourceEnvelope.Item)
	}
	privateList := requestJSON(t, api, http.MethodGet, "/public/resources", nil, nil, http.StatusOK)
	if bytes.Contains(privateList.Body.Bytes(), []byte(resourceID)) {
		t.Fatalf("private resource leaked into public listing: %s", privateList.Body.String())
	}
	published := requestJSON(t, api, http.MethodPatch, "/resources/"+resourceID, map[string]any{"title": "URL-only resource edited", "description": "integration updated", "type": "电子书", "url": "https://example.com/edited", "isPublic": true}, sessionCookie, http.StatusOK)
	if !bytes.Contains(published.Body.Bytes(), []byte("\"isPublic\":true")) {
		t.Fatalf("resource publication was not accepted: %s", published.Body.String())
	}
	publicList := requestJSON(t, api, http.MethodGet, "/public/resources", nil, nil, http.StatusOK)
	if !bytes.Contains(publicList.Body.Bytes(), []byte(resourceID)) || !bytes.Contains(publicList.Body.Bytes(), []byte("https://example.com/edited")) {
		t.Fatalf("published resource missing from public listing: %s", publicList.Body.String())
	}

	mp3Bytes := []byte("ID3sample-data-for-byte-range-preview")
	mp3 := requestJSON(t, api, http.MethodPost, "/resources", map[string]any{
		"title": "MP3 range preview", "description": "integration", "type": "音频", "filename": "range-test.mp3",
		"data": base64.StdEncoding.EncodeToString(mp3Bytes), "isPublic": false,
	}, sessionCookie, http.StatusCreated)
	var mp3Envelope map[string]map[string]any
	if err := json.Unmarshal(mp3.Body.Bytes(), &mp3Envelope); err != nil {
		t.Fatal(err)
	}
	mp3ID, _ := mp3Envelope["item"]["id"].(string)
	if mp3ID == "" {
		t.Fatalf("MP3 resource ID missing: %s", mp3.Body.String())
	}
	requestJSON(t, api, http.MethodGet, "/public/resources/"+mp3ID+"/preview", nil, nil, http.StatusNotFound)
	for _, target := range []struct {
		path   string
		cookie *http.Cookie
	}{
		{path: "/resources/" + mp3ID + "/preview", cookie: sessionCookie},
	} {
		req := httptest.NewRequest(http.MethodGet, target.path, nil)
		req.Header.Set("Range", "bytes=3-8")
		if target.cookie != nil {
			req.AddCookie(target.cookie)
		}
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		if rec.Code != http.StatusPartialContent || rec.Header().Get("Content-Range") != "bytes 3-8/"+strconv.Itoa(len(mp3Bytes)) || !bytes.Equal(rec.Body.Bytes(), mp3Bytes[3:9]) {
			t.Fatalf("range preview %s: status=%d range=%q body=%q", target.path, rec.Code, rec.Header().Get("Content-Range"), rec.Body.Bytes())
		}
		if rec.Header().Get("Content-Type") != "audio/mpeg" || !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "inline;") {
			t.Fatalf("MP3 preview headers incorrect: %#v", rec.Header())
		}
	}
	requestJSON(t, api, http.MethodPatch, "/resources/"+resourceID, map[string]any{"title": "URL-only resource edited", "description": "integration updated", "type": "电子书", "url": "https://example.com/edited", "isPublic": false}, sessionCookie, http.StatusOK)
	requestJSON(t, api, http.MethodDelete, "/resources/"+resourceID, nil, sessionCookie, http.StatusOK)
	requestJSON(t, api, http.MethodPost, "/resources/"+resourceID+"/restore", map[string]any{}, sessionCookie, http.StatusOK)
	requestJSON(t, api, http.MethodDelete, "/resources/"+resourceID, nil, sessionCookie, http.StatusOK)
	requestJSON(t, api, http.MethodDelete, "/resources/"+resourceID+"/permanent", nil, sessionCookie, http.StatusOK)
	var resourceExists, resourceAssetExists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM portal.teaching_resources WHERE resource_id=$1::uuid), EXISTS(SELECT 1 FROM portal.compliance_assets WHERE asset_path=$2)`, resourceID, "db://portal.teaching_resources/"+resourceID).Scan(&resourceExists, &resourceAssetExists); err != nil {
		t.Fatal(err)
	}
	if resourceExists || resourceAssetExists {
		t.Fatalf("permanently deleted resource must not leave a compliance record: resource=%v asset=%v", resourceExists, resourceAssetExists)
	}

	deleteTest := requestJSON(t, api, http.MethodPost, "/content", map[string]any{"title": "Permanent-delete audit test", "slug": "permanent-delete-" + randomUUID()[:8], "contentType": "article", "summary": "test", "body": "audit retention test", "visibility": "private"}, sessionCookie, http.StatusCreated)
	var deleteTestEnvelope struct {
		Item map[string]any `json:"item"`
	}
	if err := json.Unmarshal(deleteTest.Body.Bytes(), &deleteTestEnvelope); err != nil {
		t.Fatal(err)
	}
	deleteTestID, _ := deleteTestEnvelope.Item["id"].(string)
	if deleteTestID == "" {
		t.Fatal("permanent-delete test content was not created")
	}
	requestJSON(t, api, http.MethodDelete, "/content/"+deleteTestID, nil, sessionCookie, http.StatusOK)
	requestJSON(t, api, http.MethodDelete, "/content/"+deleteTestID+"/permanent", nil, sessionCookie, http.StatusOK)
	var contentExists, contentAssetExists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM portal.portal_content WHERE content_id=$1::uuid), EXISTS(SELECT 1 FROM portal.compliance_assets WHERE asset_path=$2)`, deleteTestID, "db://portal.portal_content/"+deleteTestID).Scan(&contentExists, &contentAssetExists); err != nil {
		t.Fatal(err)
	}
	if contentExists || contentAssetExists {
		t.Fatalf("permanently deleted content must not leave a compliance record: content=%v asset=%v", contentExists, contentAssetExists)
	}

	imageBytes := []byte("\x89PNG\r\n\x1a\n")
	image := requestJSON(t, api, http.MethodPost, "/uploads", map[string]any{"mime": "image/png", "data": base64.StdEncoding.EncodeToString(imageBytes)}, sessionCookie, http.StatusCreated)
	if !bytes.Contains(image.Body.Bytes(), []byte("/uploads/")) {
		t.Fatalf("image upload response contract mismatch: %s", image.Body.String())
	}
	if _, err := os.Stat(api.uploadDir); err != nil {
		t.Fatal(err)
	}
	var uploaded struct {
		URL    string `json:"url"`
		Public bool   `json:"public"`
	}
	if err := json.Unmarshal(image.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	if uploaded.Public {
		t.Fatalf("new image must default to private access: %#v", uploaded)
	}
	publicRequest := httptest.NewRequest(http.MethodGet, uploaded.URL, nil)
	publicResponse := httptest.NewRecorder()
	api.ServeHTTP(publicResponse, publicRequest)
	if publicResponse.Code != http.StatusNotFound {
		t.Fatalf("pending image should not be publicly retrievable: got %d", publicResponse.Code)
	}
	ownerRequest := httptest.NewRequest(http.MethodGet, uploaded.URL, nil)
	ownerRequest.AddCookie(sessionCookie)
	ownerResponse := httptest.NewRecorder()
	api.ServeHTTP(ownerResponse, ownerRequest)
	if ownerResponse.Code != http.StatusOK || !bytes.Equal(ownerResponse.Body.Bytes(), imageBytes) {
		t.Fatalf("owner preview should work for pending image: status=%d body=%v", ownerResponse.Code, ownerResponse.Body.Bytes())
	}

}

func requestJSON(t *testing.T, handler http.Handler, method, path string, payload any, cookie *http.Cookie, expected int) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &body)
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != expected {
		t.Fatalf("%s %s: got HTTP %d, want %d: %s", method, path, rec.Code, expected, rec.Body.String())
	}
	return rec
}
