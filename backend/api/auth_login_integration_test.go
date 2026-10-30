// auth_login_integration_test.go verifies unknown accounts retain the generic authentication response.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginUnknownAccountReturnsGenericUnauthorized(t *testing.T) {
	db := openPortalIntegrationDB(t)
	api := &portalAPI{db: db}
	email := "missing-" + strings.ReplaceAll(randomUUID(), "-", "") + "@example.invalid"
	body, err := json.Marshal(map[string]string{"email": email, "password": "valid-test-password"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(string(body)))
	response := httptest.NewRecorder()

	api.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unknown account login status = %d body=%s; want 401", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "邮箱或密码不正确") {
		t.Fatalf("unknown account response should be generic: %s", response.Body.String())
	}
}
