package main

// Compliance asset evidence and review handlers own evidence capture, verification state, and review decisions; all writes retain the shared transactional audit record.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (a *portalAPI) addComplianceEvidence(w http.ResponseWriter, r *http.Request, u *portalUser, id string) {
	var b struct {
		Type       string `json:"type"`
		URI        string `json:"uri"`
		Text       string `json:"text"`
		SHA256     string `json:"sha256"`
		Verified   bool   `json:"verified"`
		VerifiedBy string `json:"verifiedBy"`
		Notes      string `json:"notes"`
	}
	if err := decodeJSON(w, r, 128*1024, &b); err != nil {
		writeError(w, 400, "请求格式不正确。", nil)
		return
	}
	if !oneOf(b.Type, "license_text", "source_page", "purchase_receipt", "permission_email", "contract", "author_declaration", "screenshot", "hash_record", "repository_metadata", "legal_review", "other") || runeLen(strings.TrimSpace(b.URI)) > 2048 || runeLen(b.Text) > 50000 || runeLen(b.Notes) > 5000 {
		writeError(w, 400, "证据字段不正确或超出长度限制。", nil)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dbError(w, err, "无法保存证据。")
		return
	}
	defer tx.Rollback()
	var assetExists bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM portal.compliance_assets WHERE asset_id=$1)`, id).Scan(&assetExists); err != nil {
		a.dbError(w, err, "无法检查资产是否存在。")
		return
	}
	if !assetExists {
		writeError(w, 404, "资产不存在。", nil)
		return
	}
	var evidenceID string
	err = tx.QueryRowContext(r.Context(), `INSERT INTO portal.compliance_asset_evidence(asset_id,evidence_type,evidence_uri,evidence_text,sha256,verified,verified_by,verified_at,notes) VALUES($1,$2,$3,$4,$5,FALSE,'',NULL,$6) RETURNING evidence_id::text`, id, b.Type, strings.TrimSpace(b.URI), b.Text, b.SHA256, b.Notes).Scan(&evidenceID)
	if err != nil {
		a.dbError(w, err, "无法保存证据。")
		return
	}
	if err = a.logComplianceChangeTx(r.Context(), tx, r, u, "create", "asset_evidence", evidenceID, nil); err != nil {
		a.dbError(w, err, "无法写入合规审计日志，证据未提交。")
		return
	}
	if err = tx.Commit(); err != nil {
		a.dbError(w, err, "证据提交失败。")
		return
	}
	writeJSON(w, 201, map[string]any{"id": evidenceID})
}

func (a *portalAPI) addComplianceReview(w http.ResponseWriter, r *http.Request, u *portalUser, id string) {
	var b struct {
		Type            string     `json:"type"`
		Decision        string     `json:"decision"`
		RiskLevel       string     `json:"riskLevel"`
		Findings        string     `json:"findings"`
		RequiredActions []string   `json:"requiredActions"`
		FollowUpAt      *time.Time `json:"followUpAt"`
	}
	if err := decodeJSON(w, r, 64*1024, &b); err != nil {
		writeError(w, 400, "请求格式不正确。", nil)
		return
	}
	// Keep this allowlist aligned with migration 022: historical copyright reviews were converted to license reviews.
	if !oneOf(b.Type, "license", "privacy", "content", "distribution", "legal_basis", "recheck") || !oneOf(b.Decision, "approved", "approved_with_conditions", "pending_evidence", "replace", "remove", "blocked", "escalate") || !oneOf(b.RiskLevel, "low", "medium", "high", "critical", "unknown") || runeLen(b.Findings) > 10000 || len(b.RequiredActions) > 30 {
		writeError(w, 400, "审核字段不正确。", nil)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dbError(w, err, "无法保存审核记录。")
		return
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM portal.compliance_assets WHERE asset_id=$1)`, id).Scan(&exists); err != nil {
		a.dbError(w, err, "无法检查资产是否存在。")
		return
	}
	if !exists {
		writeError(w, 404, "资产不存在。", nil)
		return
	}
	actions, _ := json.Marshal(b.RequiredActions)
	var reviewID string
	err = tx.QueryRowContext(r.Context(), `INSERT INTO portal.compliance_asset_reviews(asset_id,reviewer,review_type,decision,risk_level,findings,required_actions,follow_up_at) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8) RETURNING review_id::text`, id, u.DisplayName, b.Type, b.Decision, b.RiskLevel, b.Findings, string(actions), b.FollowUpAt).Scan(&reviewID)
	if err != nil {
		a.dbError(w, err, "无法保存审核记录。")
		return
	}
	status := map[string]string{"approved": "approved", "approved_with_conditions": "approved_with_conditions", "pending_evidence": "pending", "replace": "replace_or_remove", "remove": "blocked", "blocked": "blocked", "escalate": "in_review"}[b.Decision]
	before, err := oneJSON(r.Context(), tx, `SELECT to_jsonb(x) FROM (SELECT * FROM portal.compliance_assets WHERE asset_id=$1) x`, id)
	if err != nil {
		a.dbError(w, err, "无法读取资产审核状态。")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE portal.compliance_assets SET review_status=$1,risk_level=$2,updated_at=now() WHERE asset_id=$3`, status, b.RiskLevel, id); err != nil {
		a.dbError(w, err, "审核已登记，但资产状态更新失败。")
		return
	}
	if err = a.logComplianceChangeTx(r.Context(), tx, r, u, "review", "asset", id, before); err != nil {
		a.dbError(w, err, "无法写入合规审计日志，审核变更未提交。")
		return
	}
	if err = tx.Commit(); err != nil {
		a.dbError(w, err, "审核提交失败。")
		return
	}
	writeJSON(w, 201, map[string]any{"reviewId": reviewID, "reviewStatus": status, "note": "审核记录已保存；本操作不会自动删除、下架或修改公开内容。"})
}

func (a *portalAPI) updateComplianceEvidenceVerification(w http.ResponseWriter, r *http.Request, u *portalUser, assetID, evidenceID string) {
	if !portalUUID.MatchString(assetID) || !portalUUID.MatchString(evidenceID) {
		writeError(w, http.StatusNotFound, "证据记录不存在。", nil)
		return
	}
	var b struct {
		Verified bool `json:"verified"`
	}
	if err := decodeJSON(w, r, 8*1024, &b); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确。", nil)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dbError(w, err, "读取证据记录失败。")
		return
	}
	defer tx.Rollback()
	var evidenceURI string
	if err := tx.QueryRowContext(r.Context(), "SELECT evidence_uri FROM portal.compliance_asset_evidence WHERE evidence_id=$1::uuid AND asset_id=$2::uuid", evidenceID, assetID).Scan(&evidenceURI); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "证据记录不存在。", nil)
		} else {
			a.dbError(w, err, "读取证据记录失败。")
		}
		return
	}
	if b.Verified && strings.TrimSpace(evidenceURI) == "" {
		writeError(w, http.StatusConflict, "证据没有可核验的内容或位置，不能标记为已核验。", nil)
		return
	}
	before, err := oneJSON(r.Context(), tx, "SELECT to_jsonb(x) FROM (SELECT evidence_id::text AS id,verified,verified_by,verified_at,notes FROM portal.compliance_asset_evidence WHERE evidence_id=$1::uuid) x", evidenceID)
	if err != nil {
		a.dbError(w, err, "读取证据核验状态失败。")
		return
	}
	if _, err := tx.ExecContext(r.Context(), "UPDATE portal.compliance_asset_evidence SET verified=$1,verified_by=CASE WHEN $1 THEN $2 ELSE '' END,verified_at=CASE WHEN $1 THEN now() ELSE NULL END WHERE evidence_id=$3::uuid AND asset_id=$4::uuid", b.Verified, u.DisplayName, evidenceID, assetID); err != nil {
		a.dbError(w, err, "更新证据核验状态失败。")
		return
	}
	if err := a.logComplianceChangeTx(r.Context(), tx, r, u, "verify", "asset_evidence", evidenceID, before); err != nil {
		a.dbError(w, err, "无法写入合规审计日志，证据核验变更未提交。")
		return
	}
	if err := tx.Commit(); err != nil {
		a.dbError(w, err, "证据核验变更提交失败。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"verified": b.Verified, "verifiedBy": func() string {
		if b.Verified {
			return u.DisplayName
		}
		return ""
	}(), "message": "证据核验状态已记录。审核结论仍需单独提交。"})
}
