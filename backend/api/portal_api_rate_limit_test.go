package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestRateLimitUsesForwardedClientIPOnlyBehindLoopbackProxy(t *testing.T) {
	api := newPortalAPI(nil, IPCClient{})
	for attempt := 1; attempt <= 12; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		req.RemoteAddr = "127.0.0.1:" + strconv.Itoa(60000+attempt)
		req.Header.Set("X-Portal-Client-IP", "203.0.113.10")
		if api.rateLimited(httptest.NewRecorder(), req) {
			t.Fatalf("attempt %d was limited before threshold", attempt)
		}
	}
	blocked := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.RemoteAddr = "127.0.0.1:65000"
	req.Header.Set("X-Portal-Client-IP", "203.0.113.10")
	if !api.rateLimited(blocked, req) || blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("same client IP should be limited across proxy ports: status=%d", blocked.Code)
	}

	otherClient := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	otherClient.RemoteAddr = "127.0.0.1:65001"
	otherClient.Header.Set("X-Forwarded-For", "203.0.113.11")
	if api.rateLimited(httptest.NewRecorder(), otherClient) {
		t.Fatal("different forwarded client IP should have an independent limit")
	}
}

func TestRateLimitDoesNotTrustForwardedIPFromNonLoopbackPeer(t *testing.T) {
	first := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	first.RemoteAddr = "198.51.100.7:40001"
	first.Header.Set("X-Portal-Client-IP", "203.0.113.99")
	second := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	second.RemoteAddr = "198.51.100.7:40002"
	second.Header.Set("X-Portal-Client-IP", "203.0.113.100")
	if rateLimitClientIP(first) != rateLimitClientIP(second) {
		t.Fatal("untrusted forwarded headers must not change the rate-limit client identity")
	}
}
