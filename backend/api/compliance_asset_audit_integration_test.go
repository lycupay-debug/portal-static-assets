package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestComplianceAssetWritesAndAuditShareOneTransaction exercises evidence
// capture, verification, review state changes, and the shared audit helper.
func TestComplianceAssetWritesAndAuditShareOneTransaction(t *testing.T) {
	db := openPortalIntegrationDB(t)
	actorID, _ := insertSecurityTestUser(t, db, "compliance-audit-"+strings.ReplaceAll(randomUUID(), "-", "")+"@example.invalid")
	var assetID string
	if err := db.QueryRow(
		"INSERT INTO portal.compliance_assets(asset_type,asset_name) VALUES('image',$1) RETURNING asset_id::text",
		"phase3 audit asset "+randomUUID(),
	).Scan(&assetID); err != nil {
		t.Fatalf("insert compliance asset: %v", err)
	}
	var evidenceID string
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM portal.compliance_audit_log WHERE object_id=$1::uuid OR object_id=$2::uuid", assetID, evidenceID)
		_, _ = db.Exec("DELETE FROM portal.compliance_assets WHERE asset_id=$1::uuid", assetID)
	})

	api := &portalAPI{db: db}
	actor := &portalUser{ID: actorID, DisplayName: "Compliance Integration Reviewer", Email: "reviewer@example.invalid"}

	evidenceRecorder := httptest.NewRecorder()
	evidenceRequest := httptest.NewRequest(
		http.MethodPost,
		"/compliance/assets/"+assetID+"/evidence",
		strings.NewReader(`{"type":"source_page","uri":"https://example.invalid/source","text":"source evidence","notes":"phase3 integration"}`),
	)
	api.addComplianceEvidence(evidenceRecorder, evidenceRequest, actor, assetID)
	if evidenceRecorder.Code != http.StatusCreated {
		t.Fatalf("add evidence status = %d body=%s", evidenceRecorder.Code, evidenceRecorder.Body.String())
	}
	var evidenceResponse struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(evidenceRecorder.Body.Bytes(), &evidenceResponse); err != nil {
		t.Fatalf("decode evidence response: %v", err)
	}
	evidenceID = evidenceResponse.ID
	if !portalUUID.MatchString(evidenceID) {
		t.Fatalf("evidence id = %q, want UUID", evidenceID)
	}

	var verified bool
	if err := db.QueryRow("SELECT verified FROM portal.compliance_asset_evidence WHERE evidence_id=$1::uuid", evidenceID).Scan(&verified); err != nil {
		t.Fatalf("read new evidence: %v", err)
	}
	if verified {
		t.Fatal("new evidence must not be trusted as verified on creation")
	}

	verifyRecorder := httptest.NewRecorder()
	verifyRequest := httptest.NewRequest(http.MethodPatch, "/compliance/assets/"+assetID+"/evidence/"+evidenceID, strings.NewReader(`{"verified":true}`))
	api.updateComplianceEvidenceVerification(verifyRecorder, verifyRequest, actor, assetID, evidenceID)
	if verifyRecorder.Code != http.StatusOK {
		t.Fatalf("verify evidence status = %d body=%s", verifyRecorder.Code, verifyRecorder.Body.String())
	}
	if err := db.QueryRow("SELECT verified FROM portal.compliance_asset_evidence WHERE evidence_id=$1::uuid", evidenceID).Scan(&verified); err != nil {
		t.Fatalf("read verified evidence: %v", err)
	}
	if !verified {
		t.Fatal("evidence verification state was not committed")
	}

	reviewRecorder := httptest.NewRecorder()
	reviewRequest := httptest.NewRequest(
		http.MethodPost,
		"/compliance/assets/"+assetID+"/reviews",
		strings.NewReader(`{"type":"license","decision":"approved_with_conditions","riskLevel":"medium","findings":"source reviewed","requiredActions":["confirm license scope"]}`),
	)
	api.addComplianceReview(reviewRecorder, reviewRequest, actor, assetID)
	if reviewRecorder.Code != http.StatusCreated {
		t.Fatalf("add review status = %d body=%s", reviewRecorder.Code, reviewRecorder.Body.String())
	}

	var status, risk string
	if err := db.QueryRow("SELECT review_status,risk_level FROM portal.compliance_assets WHERE asset_id=$1::uuid", assetID).Scan(&status, &risk); err != nil {
		t.Fatalf("read reviewed asset: %v", err)
	}
	if status != "approved_with_conditions" || risk != "medium" {
		t.Fatalf("asset state = %s/%s, want approved_with_conditions/medium", status, risk)
	}

	var auditCount int
	if err := db.QueryRow(
		"SELECT count(*) FROM portal.compliance_audit_log WHERE (object_type='asset_evidence' AND object_id=$1::uuid) OR (object_type='asset' AND object_id=$2::uuid)",
		evidenceID, assetID,
	).Scan(&auditCount); err != nil {
		t.Fatalf("count compliance audit entries: %v", err)
	}
	if auditCount != 3 {
		t.Fatalf("compliance audit entries = %d, want 3", auditCount)
	}
	var afterState []byte
	if err := db.QueryRow(
		"SELECT after_state FROM portal.compliance_audit_log WHERE object_type='asset' AND object_id=$1::uuid ORDER BY created_at DESC LIMIT 1",
		assetID,
	).Scan(&afterState); err != nil {
		t.Fatalf("read asset audit state: %v", err)
	}
	var auditState map[string]any
	if err := json.Unmarshal(afterState, &auditState); err != nil {
		t.Fatalf("decode asset audit state: %v", err)
	}
	if auditState["actor_display_name"] != actor.DisplayName {
		t.Fatalf("audit actor = %v, want %q", auditState["actor_display_name"], actor.DisplayName)
	}
	if auditState["request_path"] != reviewRequest.URL.Path {
		t.Fatalf("audit request path = %v, want %q", auditState["request_path"], reviewRequest.URL.Path)
	}

	var nullEvidenceURI sql.NullString
	if err := db.QueryRow("SELECT evidence_uri FROM portal.compliance_asset_evidence WHERE evidence_id=$1::uuid", evidenceID).Scan(&nullEvidenceURI); err != nil {
		t.Fatalf("verify evidence row remains readable: %v", err)
	}
	if !nullEvidenceURI.Valid || nullEvidenceURI.String == "" {
		t.Fatal("evidence URI was not preserved")
	}
}
