package main

import "testing"

func TestValidOptionalHTTPURL(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"", true},
		{"https://publisher.example/license", true},
		{"http://example.org/source", true},
		{"javascript:alert(1)", false},
		{"file:///etc/passwd", false},
		{"not a url", false},
		{"https://", false},
	} {
		if got := validOptionalHTTPURL(tc.value); got != tc.want {
			t.Errorf("validOptionalHTTPURL(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}
