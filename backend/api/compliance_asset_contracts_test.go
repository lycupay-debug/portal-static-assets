package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestComplianceAssetEvidenceRejectsInvalidInputBeforeDatabase(t *testing.T) {
	api := &portalAPI{}
	assetID := "550e8400-e29b-41d4-a716-446655440000"
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/compliance/assets/"+assetID+"/evidence", strings.NewReader(`{"type":"unsupported"}`))
	api.addComplianceEvidence(recorder, request, &portalUser{ID: "reviewer"}, assetID)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestComplianceAssetReviewRejectsInvalidDecisionBeforeDatabase(t *testing.T) {
	api := &portalAPI{}
	assetID := "550e8400-e29b-41d4-a716-446655440000"
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/compliance/assets/"+assetID+"/reviews", strings.NewReader(`{"type":"license","decision":"invalid","riskLevel":"low"}`))
	api.addComplianceReview(recorder, request, &portalUser{ID: "reviewer", DisplayName: "Reviewer"}, assetID)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestComplianceEvidenceVerificationRejectsMalformedIDsBeforeDatabase(t *testing.T) {
	api := &portalAPI{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/compliance/assets/not-a-uuid/evidence/also-invalid", strings.NewReader(`{"verified":true}`))
	api.updateComplianceEvidenceVerification(recorder, request, &portalUser{ID: "reviewer"}, "not-a-uuid", "also-invalid")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}
