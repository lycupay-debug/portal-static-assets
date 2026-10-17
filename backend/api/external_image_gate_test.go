package main

import (
	"strings"
	"testing"
)

func TestExternalImageURLPatternRecognizesHostedImageURLs(t *testing.T) {
	body := "<img src=\"https://www.snaperdigital.com/_next/image?url=%2Fimgs%2Flogo.webp&w=64&q=75\"> ![remote](https://cdn.example.invalid/test.png)"
	got := externalContentURLPattern.FindAllString(body, -1)
	if len(got) != 2 {
		t.Fatalf("expected two URLs, got %#v", got)
	}
	if got[0] != "https://www.snaperdigital.com/_next/image?url=%2Fimgs%2Flogo.webp&w=64&q=75" {
		t.Fatalf("HTML image URL was not captured exactly: %q", got[0])
	}
	if !strings.Contains(got[0], "/_next/image") || !externalImageFilePattern.MatchString(got[1]) {
		t.Fatalf("image URL heuristics did not recognize external images: %#v", got)
	}
}
