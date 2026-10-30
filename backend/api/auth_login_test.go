// auth_login_test.go verifies authentication distinguishes bad credentials from database failures.

package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginReportsServiceFailureWhenDatabaseIsUnavailable(t *testing.T) {
	db, err := sql.Open("pgx", "host=127.0.0.1 port=1 user=unused dbname=unused sslmode=disable connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	api := &portalAPI{db: db}
	request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"email":"test@example.com","password":"valid-test-password"}`))
	response := httptest.NewRecorder()

	api.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("login status = %d body=%s; want 500 when the database is unavailable", response.Code, response.Body.String())
	}
}
