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
		_, _ = db.ExecContext(cleanCtx, `DELETE FROM portal.portal_content WHERE author_id=$1`, userID)
		_, _ = db.ExecContext(cleanCtx, `DELETE FROM portal.teaching_resources WHERE owner_id=$1`, userID)
		_, _ = db.ExecContext(cleanCtx, `DELETE FROM portal.portal_auth_sessions WHERE user_id=$1`, userID)
		_, _ = db.ExecContext(cleanCtx, `DELETE FROM portal.portal_users WHERE user_id=$1`, userID)
	}()

	api := newPortalAPI(db, IPCClient{SocketPath: t.TempDir() + "/not-running.sock", Timeout: 100 * time.Millisecond})
	api.uploadDir = t.TempDir()
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

	resource := requestJSON(t, api, http.MethodPost, "/resources", map[string]any{"title": "URL-only resource", "description": "integration", "type": "电子书", "url": "https://example.com/material"}, sessionCookie, http.StatusCreated)
	var resourceEnvelope struct {
		Item map[string]any `json:"item"`
	}
	_ = json.Unmarshal(resource.Body.Bytes(), &resourceEnvelope)
	resourceID, _ := resourceEnvelope.Item["id"].(string)
	if resourceID == "" || resourceEnvelope.Item["url"] != "https://example.com/material" || resourceEnvelope.Item["type"] != "电子书" {
		t.Fatalf("resource response contract mismatch: %#v", resourceEnvelope.Item)
	}
	updated := requestJSON(t, api, http.MethodPatch, "/resources/"+resourceID, map[string]any{"title": "URL-only resource edited", "description": "integration updated", "type": "电子书", "url": "https://example.com/edited"}, sessionCookie, http.StatusOK)
	if !bytes.Contains(updated.Body.Bytes(), []byte("URL-only resource edited")) {
		t.Fatalf("resource edit failed: %s", updated.Body.String())
	}
	requestJSON(t, api, http.MethodDelete, "/resources/"+resourceID, nil, sessionCookie, http.StatusOK)
	requestJSON(t, api, http.MethodPost, "/resources/"+resourceID+"/restore", map[string]any{}, sessionCookie, http.StatusOK)
	requestJSON(t, api, http.MethodDelete, "/resources/"+resourceID, nil, sessionCookie, http.StatusOK)
	requestJSON(t, api, http.MethodDelete, "/resources/"+resourceID+"/permanent", nil, sessionCookie, http.StatusOK)

	imageBytes := []byte("\x89PNG\r\n\x1a\n")
	image := requestJSON(t, api, http.MethodPost, "/uploads", map[string]any{"mime": "image/png", "data": base64.StdEncoding.EncodeToString(imageBytes)}, sessionCookie, http.StatusCreated)
	if !bytes.Contains(image.Body.Bytes(), []byte("/uploads/")) {
		t.Fatalf("image upload response contract mismatch: %s", image.Body.String())
	}
	if _, err := os.Stat(api.uploadDir); err != nil {
		t.Fatal(err)
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
