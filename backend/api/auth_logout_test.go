// auth_logout_test.go verifies logout reports a failed server-side revocation instead of false success.

package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogoutReportsFailureWhenSessionRevocationFails(t *testing.T) {
	db, err := sql.Open("pgx", "host=127.0.0.1 port=1 user=unused dbname=unused sslmode=disable connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	api := &portalAPI{db: db}
	request := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: portalCookie, Value: "test-session-token"})
	response := httptest.NewRecorder()

	api.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("logout status = %d body=%s; want 503 when server-side revocation fails", response.Code, response.Body.String())
	}
	cookies := response.Header().Values("Set-Cookie")
	if len(cookies) != 2 || !strings.Contains(cookies[0], "Max-Age=0") || !strings.Contains(cookies[1], "Max-Age=0") {
		t.Fatalf("logout must clear both cookie paths even when revocation fails: %v", cookies)
	}
}
