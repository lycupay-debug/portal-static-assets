package main

import (
	"database/sql"
	"net/http"
	"strings"
)

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
