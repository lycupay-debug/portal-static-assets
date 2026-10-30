// image_cookie_path_test.go verifies authenticated image URLs are covered by the session cookie path.

package main

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"
)

func TestSessionCookieIsSentToAPIImagePath(t *testing.T) {
	response := http.Header{}
	writer := &headerResponseWriter{header: response}
	setSessionCookie(writer, "test-session-token", 3600)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	loginURL, err := url.Parse("https://portal.example/api/auth/login")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(loginURL, (&http.Response{Header: response}).Cookies())

	apiImageURL, err := url.Parse("https://portal.example/api/uploads/00000000-0000-4000-8000-000000000000.png")
	if err != nil {
		t.Fatal(err)
	}
	if got := len(jar.Cookies(apiImageURL)); got != 1 {
		t.Fatalf("API-prefixed image request received %d session cookies; want 1", got)
	}

	legacyImageURL, err := url.Parse("https://portal.example/uploads/00000000-0000-4000-8000-000000000000.png")
	if err != nil {
		t.Fatal(err)
	}
	if got := len(jar.Cookies(legacyImageURL)); got != 0 {
		t.Fatalf("legacy root image request received %d session cookies; want 0 before its Caddy redirect", got)
	}
}

type headerResponseWriter struct {
	header http.Header
}

func (w *headerResponseWriter) Header() http.Header       { return w.header }
func (w *headerResponseWriter) Write([]byte) (int, error) { return 0, nil }
func (w *headerResponseWriter) WriteHeader(int)           {}
