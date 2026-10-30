package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// This file exercises pure backend contracts and HTTP boundary behavior without
// requiring PostgreSQL. Database transaction and concurrency behavior remains
// an integration-test responsibility.
type stubEventStore struct {
	duplicate bool
	err       error
	calls     int
	event     EventInput
}

func (s *stubEventStore) ReceiveEvent(_ context.Context, event EventInput) (bool, error) {
	s.calls++
	s.event = event
	return s.duplicate, s.err
}

func TestValidateEvent(t *testing.T) {
	tests := []struct {
		name  string
		event EventInput
		valid bool
	}{
		{"valid object", EventInput{EventID: "550e8400-e29b-41d4-a716-446655440000", EventType: "portal.content.updated", Payload: json.RawMessage(`{"id":"123"}`)}, true},
		{"invalid uuid", EventInput{EventID: "not-a-uuid", EventType: "portal.content.updated", Payload: json.RawMessage(`{}`)}, false},
		{"invalid event type", EventInput{EventID: "550e8400-e29b-41d4-a716-446655440000", EventType: "Portal.Content", Payload: json.RawMessage(`{}`)}, false},
		{"empty payload", EventInput{EventID: "550e8400-e29b-41d4-a716-446655440000", EventType: "portal.content.updated"}, false},
		{"invalid json", EventInput{EventID: "550e8400-e29b-41d4-a716-446655440000", EventType: "portal.content.updated", Payload: json.RawMessage(`{"x":`)}, false},
		{"array payload", EventInput{EventID: "550e8400-e29b-41d4-a716-446655440000", EventType: "portal.content.updated", Payload: json.RawMessage(`[]`)}, false},
		{"scalar payload", EventInput{EventID: "550e8400-e29b-41d4-a716-446655440000", EventType: "portal.content.updated", Payload: json.RawMessage(`"value"`)}, false},
		{"null payload", EventInput{EventID: "550e8400-e29b-41d4-a716-446655440000", EventType: "portal.content.updated", Payload: json.RawMessage(`null`)}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateEvent(test.event)
			if (err == nil) != test.valid {
				t.Fatalf("validateEvent() error = %v, want valid=%t", err, test.valid)
			}
		})
	}
}

func TestReceiveEventHandlerRejectsPrivilegedEventBeforePersistence(t *testing.T) {
	store := &stubEventStore{}
	body := `{"event_id":"550e8400-e29b-41d4-a716-446655440000","event_type":"portal.command.execute","payload":{"task_id":"x"}}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(body))

	receiveEventHandler(store).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if store.calls != 0 {
		t.Fatalf("store called %d times; retired command event must be rejected before persistence", store.calls)
	}
}

func TestReceiveEventHandlerReportsDuplicateAndConflict(t *testing.T) {
	const body = `{"event_id":"550e8400-e29b-41d4-a716-446655440000","event_type":"portal.content.updated","payload":{"id":"123"}}`
	tests := []struct {
		name          string
		duplicate     bool
		err           error
		wantStatus    int
		wantDuplicate string
	}{
		{"new event", false, nil, http.StatusAccepted, `"duplicate":false`},
		{"identical duplicate", true, nil, http.StatusOK, `"duplicate":true`},
		{"conflicting duplicate", false, errEventConflict, http.StatusConflict, ""},
		{"storage unavailable", false, errors.New("database unavailable"), http.StatusServiceUnavailable, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &stubEventStore{duplicate: test.duplicate, err: test.err}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(body))

			receiveEventHandler(store).ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if store.calls != 1 {
				t.Fatalf("store calls = %d, want 1", store.calls)
			}
			if test.wantDuplicate != "" && !strings.Contains(recorder.Body.String(), test.wantDuplicate) {
				t.Fatalf("response %q does not contain %q", recorder.Body.String(), test.wantDuplicate)
			}
		})
	}
}

func TestReceiveEventHandlerRejectsTrailingJSON(t *testing.T) {
	store := &stubEventStore{}
	body := `{"event_id":"550e8400-e29b-41d4-a716-446655440000","event_type":"portal.content.updated","payload":{}} {}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(body))

	receiveEventHandler(store).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if store.calls != 0 {
		t.Fatalf("store called %d times for malformed request", store.calls)
	}
}

func TestDecodeJSONRejectsUnknownTrailingAndOversizedInput(t *testing.T) {
	type requestBody struct {
		Name string `json:"name"`
	}
	tests := []struct {
		name string
		body string
		max  int64
		ok   bool
	}{
		{"valid", `{"name":"portal"}`, 64, true},
		{"unknown field", `{"name":"portal","admin":true}`, 64, false},
		{"trailing value", `{"name":"portal"} {}`, 64, false},
		{"oversized", `{"name":"` + strings.Repeat("x", 128) + `"}`, 32, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			var decoded requestBody
			err := decodeJSON(recorder, request, test.max, &decoded)
			if (err == nil) != test.ok {
				t.Fatalf("decodeJSON() error = %v, want success=%t", err, test.ok)
			}
		})
	}
}

func TestComplianceReviewerEndpointRejectsStrictJSONViolations(t *testing.T) {
	const reviewerID = "550e8400-e29b-41d4-a716-446655440000"
	validPrefix := `{"isCopyrightReviewer":true}`
	tests := []struct {
		name string
		body string
	}{
		{"unknown field", `{"isCopyrightReviewer":true,"isAdmin":true}`},
		{"trailing JSON value", validPrefix + ` {}`},
		{"oversized trailing data", validPrefix + strings.Repeat(" ", 4*1024)},
		{"missing field", `{"other":true}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := &portalAPI{}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPatch, "/compliance/reviewers/"+reviewerID, strings.NewReader(test.body))
			api.complianceReviewerUsers(recorder, request, &portalUser{ID: "admin", Email: "admin@example.invalid"}, "/"+reviewerID)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
		})
	}
}

func TestRBACMutationEndpointsRejectStrictJSONViolations(t *testing.T) {
	const userID = "550e8400-e29b-41d4-a716-446655440000"
	tests := []struct {
		name     string
		userBody string
		roleBody string
	}{
		{
			name:     "unknown fields",
			userBody: `{"roles":["user"],"isAdmin":true}`,
			roleBody: `{"permissions":[],"isAdmin":true}`,
		},
		{
			name:     "trailing JSON values",
			userBody: `{"roles":["user"]} {}`,
			roleBody: `{"permissions":[]} {}`,
		},
		{
			name:     "oversized trailing data",
			userBody: `{"roles":["user"]}` + strings.Repeat(" ", 16*1024),
			roleBody: `{"permissions":[]}` + strings.Repeat(" ", 16*1024),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := &portalAPI{}
			actor := &portalUser{ID: userID, Email: "admin@example.invalid"}

			userRecorder := httptest.NewRecorder()
			userRequest := httptest.NewRequest(http.MethodPatch, "/admin/users/"+userID+"/roles", strings.NewReader(test.userBody))
			api.updateUserRoles(userRecorder, userRequest, actor, userID)
			if userRecorder.Code != http.StatusBadRequest {
				t.Fatalf("user role update status = %d, want %d; body=%s", userRecorder.Code, http.StatusBadRequest, userRecorder.Body.String())
			}

			roleRecorder := httptest.NewRecorder()
			roleRequest := httptest.NewRequest(http.MethodPatch, "/admin/roles/user/permissions", strings.NewReader(test.roleBody))
			api.updateRolePermissions(roleRecorder, roleRequest, actor, "user")
			if roleRecorder.Code != http.StatusBadRequest {
				t.Fatalf("role permission update status = %d, want %d; body=%s", roleRecorder.Code, http.StatusBadRequest, roleRecorder.Body.String())
			}
		})
	}
}

func TestAddComplianceReviewRejectsRemovedCopyrightReviewType(t *testing.T) {
	const assetID = "550e8400-e29b-41d4-a716-446655440000"
	api := &portalAPI{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/compliance/assets/"+assetID+"/reviews", strings.NewReader(`{"type":"copyright","decision":"approved","riskLevel":"low"}`))
	api.addComplianceReview(recorder, request, &portalUser{ID: "admin", Email: "admin@example.invalid"}, assetID)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestRateLimitMapCapacityBoundsMemory(t *testing.T) {
	now := time.Now()
	request := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	request.RemoteAddr = "192.0.2.1:1234"

	full := &portalAPI{rate: make(map[string]rateEntry, maxRateLimitEntries)}
	for i := 0; i < maxRateLimitEntries; i++ {
		full.rate["active-"+string(rune(i+1))] = rateEntry{count: 1, until: now.Add(time.Minute)}
	}
	recorder := httptest.NewRecorder()
	if !full.rateLimited(recorder, request) {
		t.Fatal("new client key should be rejected when the active key map is full")
	}
	if recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") != "60" {
		t.Fatalf("capacity response = status %d retry-after %q", recorder.Code, recorder.Header().Get("Retry-After"))
	}
	if got := len(full.rate); got != maxRateLimitEntries {
		t.Fatalf("rate map grew to %d entries, want cap %d", got, maxRateLimitEntries)
	}

	withExpired := &portalAPI{rate: make(map[string]rateEntry, maxRateLimitEntries)}
	for i := 0; i < maxRateLimitEntries; i++ {
		until := now.Add(time.Minute)
		if i == 0 {
			until = now.Add(-time.Second)
		}
		withExpired.rate["entry-"+string(rune(i+1))] = rateEntry{count: 1, until: until}
	}
	recorder = httptest.NewRecorder()
	if withExpired.rateLimited(recorder, request) {
		t.Fatalf("expired entry should be reclaimed to admit a new client: %s", recorder.Body.String())
	}
	if got := len(withExpired.rate); got != maxRateLimitEntries {
		t.Fatalf("rate map size after expired-entry reclamation = %d, want %d", got, maxRateLimitEntries)
	}
	if _, ok := withExpired.rate["192.0.2.1:/auth/login"]; !ok {
		t.Fatal("new client key was not recorded after expired-entry reclamation")
	}
}

func TestRateLimitClientIPTrustBoundary(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		portalIP   string
		forwarded  string
		want       string
	}{
		{"loopback custom proxy header", "127.0.0.1:8080", "198.51.100.7", "", "198.51.100.7"},
		{"generic forwarded header ignored", "127.0.0.1:8080", "", "198.51.100.8", "127.0.0.1"},
		{"direct remote ignores proxy header", "198.51.100.9:1234", "198.51.100.7", "198.51.100.8", "198.51.100.9"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = test.remoteAddr
			if test.portalIP != "" {
				request.Header.Set("X-Portal-Client-IP", test.portalIP)
			}
			if test.forwarded != "" {
				request.Header.Set("X-Forwarded-For", test.forwarded)
			}
			if got := rateLimitClientIP(request); got != test.want {
				t.Fatalf("rateLimitClientIP() = %q, want %q", got, test.want)
			}
		})
	}
}
