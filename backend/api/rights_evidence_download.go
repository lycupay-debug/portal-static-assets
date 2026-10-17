package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	var evidenceURI string
	if err := a.db.QueryRowContext(r.Context(), "SELECT evidence_uri FROM portal.compliance_asset_evidence WHERE evidence_id=$1::uuid AND asset_id=$2::uuid", evidenceID, assetID).Scan(&evidenceURI); err != nil {
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
	before, _ := oneJSON(r.Context(), a.db, "SELECT to_jsonb(x) FROM (SELECT evidence_id::text AS id,verified,verified_by,verified_at,notes FROM portal.compliance_asset_evidence WHERE evidence_id=$1::uuid) x", evidenceID)
	if _, err := a.db.ExecContext(r.Context(), "UPDATE portal.compliance_asset_evidence SET verified=$1,verified_by=CASE WHEN $1 THEN $2 ELSE '' END,verified_at=CASE WHEN $1 THEN now() ELSE NULL END WHERE evidence_id=$3::uuid AND asset_id=$4::uuid", b.Verified, u.DisplayName, evidenceID, assetID); err != nil {
		a.dbError(w, err, "更新证据核验状态失败。")
		return
	}
	a.logComplianceChange(r, u, "verify", "asset_evidence", evidenceID, before)
	writeJSON(w, http.StatusOK, map[string]any{"verified": b.Verified, "verifiedBy": func() string {
		if b.Verified {
			return u.DisplayName
		}
		return ""
	}(), "message": "证据核验状态已记录。审核结论仍需单独提交。"})
}

func (a *portalAPI) downloadComplianceEvidence(w http.ResponseWriter, r *http.Request, assetID, evidenceID string) {
	if !portalUUID.MatchString(assetID) || !portalUUID.MatchString(evidenceID) {
		writeError(w, http.StatusNotFound, "证据文件不存在。", nil)
		return
	}
	var storedName, originalName string
	err := a.db.QueryRowContext(r.Context(), "SELECT COALESCE(metadata->>'storagePath',''),COALESCE(metadata->>'originalFilename','') FROM portal.compliance_asset_evidence WHERE evidence_id=$1::uuid AND asset_id=$2::uuid", evidenceID, assetID).Scan(&storedName, &originalName)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "证据文件不存在。", nil)
		} else {
			a.dbError(w, err, "读取证据文件记录失败。")
		}
		return
	}
	if storedName == "" || filepath.Base(storedName) != storedName || !strings.HasPrefix(storedName, "rights-evidence-") {
		writeError(w, http.StatusNotFound, "该证据记录没有可下载的附件。", nil)
		return
	}
	f, err := os.Open(filepath.Join(a.resourceDir, storedName))
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "证据附件文件不存在，请联系管理员核查存储记录。", nil)
		} else {
			a.dbError(w, err, "打开证据附件失败。")
		}
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		a.dbError(w, err, "读取证据附件信息失败。")
		return
	}
	if originalName == "" {
		originalName = storedName
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(filepath.Base(originalName))))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	http.ServeContent(w, r, filepath.Base(originalName), info.ModTime(), f)
}
