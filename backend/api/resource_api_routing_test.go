package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResourceRoutesRequireSessionBeforeDispatch(t *testing.T) {
	api := &portalAPI{}
	request := httptest.NewRequest(http.MethodGet, "https://portal.example/resources", nil)
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated resource request status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestResourceRouteDispatchRejectsMalformedIDsAndUnknownActions(t *testing.T) {
	db := openManagedFileTestDB(t, &managedFileTestConnector{})
	api := &portalAPI{db: db, searchCache: newSearchResultCache()}
	validID := "00000000-0000-4000-8000-000000000003"
	cases := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{name: "malformed id", method: http.MethodGet, path: "/resources/not-a-uuid", want: http.StatusNotFound},
		{name: "unknown action", method: http.MethodGet, path: "/resources/" + validID + "/unknown", want: http.StatusNotFound},
		{name: "action with wrong method", method: http.MethodPost, path: "/resources/" + validID + "/download", want: http.StatusNotFound},
		{name: "unsupported item method", method: http.MethodPut, path: "/resources/" + validID, want: http.StatusMethodNotAllowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := managedFileTestRequest(api, tc.method, tc.path, "")
			if response.Code != tc.want {
				t.Fatalf("status = %d body=%s, want %d", response.Code, response.Body.String(), tc.want)
			}
		})
	}
}
