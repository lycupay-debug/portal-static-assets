package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateContentAcceptsValidDraft(t *testing.T) {
	result := validateContent(contentValidationInput{
		Title: "A Useful Article", Slug: "a-useful-article", ContentType: "article",
		Visibility: "public", Summary: "A short summary", Body: "Body text",
	})
	if !result.Valid || len(result.Errors) != 0 {
		t.Fatalf("validateContent() = %#v, want valid result", result)
	}
}

func TestValidateContentReportsDeterministicErrors(t *testing.T) {
	result := validateContent(contentValidationInput{Slug: "Bad--Slug", ContentType: "unknown", Visibility: "everyone"})
	if result.Valid || len(result.Errors) < 5 {
		t.Fatalf("validateContent() = %#v, expected multiple validation errors", result)
	}
}

func TestValidateContentCountsUnicodeCharacters(t *testing.T) {
	input := contentValidationInput{
		Title: strings.Repeat("界", 201), Slug: "valid-slug", ContentType: "article",
		Visibility: "private", Body: "正文",
	}
	result := validateContent(input)
	if result.Valid || !strings.Contains(strings.Join(result.Errors, "\n"), "title must be at most 200 characters") {
		t.Fatalf("validateContent() = %#v, expected Unicode-aware title length failure", result)
	}
}

func TestValidateContentEventInvalidDraftIsSuccessfulExecution(t *testing.T) {
	resultJSON, err := validateContentEvent(context.Background(), EventInput{
		EventID: "018f2f5e-7a2b-7c4d-8e9f-0123456789ac", EventType: contentValidationEventType,
		Payload: json.RawMessage(`{"title":"","slug":"Bad Slug","content_type":"article","visibility":"private","body":"body"}`),
	})
	if err != nil {
		t.Fatalf("invalid business input should return a result, not a retryable error: %v", err)
	}
	var result contentValidationResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		t.Fatal(err)
	}
	if result.Valid || len(result.Errors) == 0 {
		t.Fatalf("result = %#v, expected invalid content result", result)
	}
}

func TestValidateContentEventAcceptsPublishedContentPayloadWithTags(t *testing.T) {
	resultJSON, err := validateContentEvent(context.Background(), EventInput{
		Payload: json.RawMessage(`{"title":"Published article","slug":"published-article","content_type":"article","tags":["编程","Python"],"visibility":"public","summary":"Summary","body":"Body"}`),
	})
	if err != nil {
		t.Fatalf("published content payload should produce a validation result: %v", err)
	}
	var result contentValidationResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Valid || len(result.Errors) != 0 {
		t.Fatalf("published content payload with approved tags must be valid, got %#v", result)
	}
}

func TestValidateContentEventRejectsUnapprovedTags(t *testing.T) {
	resultJSON, err := validateContentEvent(context.Background(), EventInput{
		Payload: json.RawMessage(`{"title":"Published article","slug":"published-article","content_type":"article","tags":["not-a-real-tag"],"visibility":"public","body":"Body"}`),
	})
	if err != nil {
		t.Fatalf("invalid tags should return a validation result, not a retryable error: %v", err)
	}
	var result contentValidationResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		t.Fatal(err)
	}
	if result.Valid || !strings.Contains(strings.Join(result.Errors, "\\n"), "approved Knowledge Plaza tags") {
		t.Fatalf("unapproved tag should be rejected, got %#v", result)
	}
}

func TestValidateContentEventRejectsUnknownFields(t *testing.T) {
	resultJSON, err := validateContentEvent(context.Background(), EventInput{
		Payload: json.RawMessage(`{"title":"Title","slug":"title","content_type":"article","visibility":"public","body":"Body","is_admin":true}`),
	})
	if err != nil {
		t.Fatalf("permanent payload errors should not be retried: %v", err)
	}
	var result contentValidationResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		t.Fatal(err)
	}
	if result.Valid || len(result.Errors) == 0 {
		t.Fatalf("result = %#v, expected invalid payload result", result)
	}
}

func TestResourceGradeUsesCanonicalGradeTaxonomy(t *testing.T) {
	for _, grade := range portalTaxonomy["年级"] {
		if got := resourceGrade(grade); got != grade {
			t.Errorf("resourceGrade(%q) = %q, want same canonical grade", grade, got)
		}
	}
	if got := resourceGrade(""); got != "" {
		t.Errorf("resourceGrade(empty) = %q, want empty for unrestricted grade", got)
	}
	if got := resourceGrade("幼儿园"); got != "__invalid_grade__" {
		t.Errorf("resourceGrade(unsupported) = %q, want invalid sentinel", got)
	}
}
