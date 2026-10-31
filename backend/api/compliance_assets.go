package main

// Compliance asset registry handlers own asset listing, detail routing, and audited metadata updates.

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (a *portalAPI) listComplianceAssets(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	typ := q.Get("type")
	status := q.Get("status")
	risk := q.Get("risk")
	search := strings.TrimSpace(q.Get("q"))
	if typ != "" && !oneOf(typ, "image", "icon", "font", "audio", "video", "software_dependency", "source_file", "policy_document", "other") {
		writeError(w, 400, "资产类型筛选不正确。", nil)
		return
	}
	if status != "" && !oneOf(status, "pending", "in_review", "approved", "approved_with_conditions", "replace_or_remove", "blocked") {
		writeError(w, 400, "审核状态筛选不正确。", nil)
		return
	}
	if risk != "" && !oneOf(risk, "low", "medium", "high", "critical", "unknown") {
		writeError(w, 400, "风险等级筛选不正确。", nil)
		return
	}
	limit := 100
	if raw := q.Get("limit"); raw != "" {
		if n, e := strconv.Atoi(raw); e == nil && n > 0 {
			limit = n
		}
	}
	if limit > 500 {
		limit = 500
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	if offset < 0 {
		offset = 0
	}
	if offset > 100000 {
		offset = 100000
	}
	base := ` FROM portal.compliance_assets WHERE COALESCE(metadata->>'superseded','false') <> 'true' AND ($1='' OR asset_type=$1) AND ($2='' OR review_status=$2) AND ($3='' OR risk_level=$3) AND ($4='' OR asset_name ILIKE '%'||$4||'%' OR source_url ILIKE '%'||$4||'%' OR asset_path ILIKE '%'||$4||'%')`
	var total int
	if err := a.db.QueryRowContext(r.Context(), `SELECT count(*)`+base, typ, status, risk, search).Scan(&total); err != nil {
		a.dbError(w, err, "无法读取资产总数。")
		return
	}
	items, err := queryJSONRows(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT asset_id::text AS id,asset_type AS type,asset_name AS name,asset_path AS path,source_url AS "sourceUrl",creator_or_rightsholder AS "rightsholder",ownership_basis AS "ownershipBasis",license_id::text AS "licenseId",permission_status AS "permissionStatus",use_scope AS "useScope",distribution_status AS "distributionStatus",review_status AS "reviewStatus",risk_level AS "riskLevel",sha256,"file_size_bytes" AS "fileSizeBytes",discovered_by AS "discoveredBy",first_discovered_at AS "firstDiscoveredAt",last_scanned_at AS "lastScannedAt",published_or_active AS "active",metadata,notes,updated_at AS "updatedAt"`+base+` ORDER BY CASE risk_level WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'unknown' THEN 3 ELSE 4 END,updated_at DESC LIMIT $5 OFFSET $6) x`, typ, status, risk, search, limit, offset)
	if err != nil {
		a.dbError(w, err, "无法读取资产台账。")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

func (a *portalAPI) complianceAssetByID(w http.ResponseWriter, r *http.Request, u *portalUser, tail string) {
	parts := strings.Split(tail, "/")
	id := parts[0]
	if !portalUUID.MatchString(id) {
		writeError(w, 404, "资产不存在。", nil)
		return
	}
	if len(parts) == 2 && parts[1] == "evidence" && r.Method == http.MethodPost {
		a.addComplianceEvidence(w, r, u, id)
		return
	}
	if len(parts) == 3 && parts[1] == "evidence" && r.Method == http.MethodPatch {
		a.updateComplianceEvidenceVerification(w, r, u, id, parts[2])
		return
	}
	if len(parts) == 2 && parts[1] == "reviews" && r.Method == http.MethodPost {
		a.addComplianceReview(w, r, u, id)
		return
	}
	if len(parts) != 1 {
		writeError(w, 404, "资产接口不存在。", nil)
		return
	}
	if r.Method == http.MethodGet {
		asset, err := oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT * FROM portal.compliance_assets WHERE asset_id=$1) x`, id)
		if err != nil {
			if err == sql.ErrNoRows {
				writeError(w, 404, "资产不存在。", nil)
			} else {
				a.dbError(w, err, "无法读取资产详情。")
			}
			return
		}
		evidence, err := queryJSONRows(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT evidence_id::text AS id,evidence_type AS type,evidence_uri AS uri,evidence_text AS text,sha256,verified,verified_by AS "verifiedBy",verified_at AS "verifiedAt",captured_at AS "capturedAt",metadata,notes FROM portal.compliance_asset_evidence WHERE asset_id=$1 ORDER BY captured_at DESC LIMIT 100) x`, id)
		if err != nil {
			a.dbError(w, err, "无法读取证据记录。")
			return
		}
		reviews, err := queryJSONRows(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT review_id::text AS id,reviewer,review_type AS type,decision,risk_level AS "riskLevel",findings,required_actions AS "requiredActions",evidence_snapshot AS "evidenceSnapshot",reviewed_at AS "reviewedAt",follow_up_at AS "followUpAt" FROM portal.compliance_asset_reviews WHERE asset_id=$1 ORDER BY reviewed_at DESC LIMIT 100) x`, id)
		if err != nil {
			a.dbError(w, err, "无法读取审核历史。")
			return
		}
		writeJSON(w, 200, map[string]any{"item": asset, "evidence": evidence, "reviews": reviews})
		return
	}
	if r.Method == http.MethodPatch || r.Method == http.MethodPut {
		a.updateComplianceAsset(w, r, u, id)
		return
	}
	writeError(w, 405, "不支持的请求方式。", nil)
}

func (a *portalAPI) updateComplianceAsset(w http.ResponseWriter, r *http.Request, u *portalUser, id string) {
	var b struct {
		ReviewStatus       *string `json:"reviewStatus"`
		RiskLevel          *string `json:"riskLevel"`
		OwnershipBasis     *string `json:"ownershipBasis"`
		PermissionStatus   *string `json:"permissionStatus"`
		UseScope           *string `json:"useScope"`
		DistributionStatus *string `json:"distributionStatus"`
		SourceURL          *string `json:"sourceUrl"`
		Rightsholder       *string `json:"rightsholder"`
		LicenseID          *string `json:"licenseId"`
		Notes              *string `json:"notes"`
		PublishedOrActive  *bool   `json:"active"`
	}
	if err := decodeJSON(w, r, 32*1024, &b); err != nil {
		writeError(w, 400, "请求格式不正确。", nil)
		return
	}
	if b.ReviewStatus != nil && !oneOf(*b.ReviewStatus, "pending", "in_review", "approved", "approved_with_conditions", "replace_or_remove", "blocked") {
		writeError(w, 400, "审核状态不正确。", nil)
		return
	}
	if b.RiskLevel != nil && !oneOf(*b.RiskLevel, "low", "medium", "high", "critical", "unknown") {
		writeError(w, 400, "风险等级不正确。", nil)
		return
	}
	if b.OwnershipBasis != nil && !oneOf(*b.OwnershipBasis, "original", "commissioned", "licensed", "open_license", "public_domain", "user_submitted", "third_party_permission", "unknown") {
		writeError(w, 400, "权利归属类型不正确。", nil)
		return
	}
	if b.PermissionStatus != nil && !oneOf(*b.PermissionStatus, "confirmed", "conditional", "unknown", "not_permitted", "not_applicable") {
		writeError(w, 400, "授权状态不正确。", nil)
		return
	}
	if b.UseScope != nil && !oneOf(*b.UseScope, "commercial_web", "internal_only", "redistribution", "modification", "multiple", "unknown") {
		writeError(w, 400, "使用范围不正确。", nil)
		return
	}
	if b.DistributionStatus != nil && !oneOf(*b.DistributionStatus, "served_publicly", "bundled_in_frontend", "server_side_only", "internal_only", "not_distributed", "unknown") {
		writeError(w, 400, "分发状态不正确。", nil)
		return
	}
	if b.LicenseID != nil && strings.TrimSpace(*b.LicenseID) != "" && !portalUUID.MatchString(strings.TrimSpace(*b.LicenseID)) {
		writeError(w, 400, "许可证目录 ID 不正确。", nil)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dbError(w, err, "无法更新资产台账。")
		return
	}
	defer tx.Rollback()
	before, err := oneJSON(r.Context(), tx, `SELECT to_jsonb(x) FROM (SELECT * FROM portal.compliance_assets WHERE asset_id=$1) x`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, 404, "资产不存在。", nil)
		} else {
			a.dbError(w, err, "无法读取资产。")
		}
		return
	}
	sets := []string{"updated_at=now()"}
	args := []any{}
	add := func(col string, val any) {
		args = append(args, val)
		sets = append(sets, fmt.Sprintf("%s=$%d", col, len(args)))
	}
	if b.ReviewStatus != nil {
		add("review_status", *b.ReviewStatus)
	}
	if b.RiskLevel != nil {
		add("risk_level", *b.RiskLevel)
	}
	if b.OwnershipBasis != nil {
		add("ownership_basis", *b.OwnershipBasis)
	}
	if b.PermissionStatus != nil {
		add("permission_status", *b.PermissionStatus)
	}
	if b.UseScope != nil {
		add("use_scope", *b.UseScope)
	}
	if b.DistributionStatus != nil {
		add("distribution_status", *b.DistributionStatus)
	}
	if b.SourceURL != nil {
		add("source_url", strings.TrimSpace(*b.SourceURL))
	}
	if b.Rightsholder != nil {
		add("creator_or_rightsholder", strings.TrimSpace(*b.Rightsholder))
	}
	if b.LicenseID != nil {
		if strings.TrimSpace(*b.LicenseID) == "" {
			add("license_id", nil)
		} else {
			add("license_id", strings.TrimSpace(*b.LicenseID))
		}
	}
	if b.Notes != nil {
		add("notes", *b.Notes)
	}
	if b.PublishedOrActive != nil {
		add("published_or_active", *b.PublishedOrActive)
	}
	if len(args) == 0 {
		writeError(w, 400, "没有可更新的字段。", nil)
		return
	}
	args = append(args, id)
	// #nosec G701 G202 -- every column identifier is a compile-time literal; request data is passed only as $n parameters.
	if _, err = tx.ExecContext(r.Context(), `UPDATE portal.compliance_assets SET `+strings.Join(sets, ",")+fmt.Sprintf(" WHERE asset_id=$%d", len(args)), args...); err != nil {
		a.dbError(w, err, "无法更新资产台账。")
		return
	}
	if err = a.logComplianceChangeTx(r.Context(), tx, r, u, "update", "asset", id, before); err != nil {
		a.dbError(w, err, "无法写入合规审计日志，资产变更未提交。")
		return
	}
	item, err := oneJSON(r.Context(), tx, `SELECT to_jsonb(x) FROM (SELECT * FROM portal.compliance_assets WHERE asset_id=$1) x`, id)
	if err != nil {
		a.dbError(w, err, "资产已更新，但读取结果失败。")
		return
	}
	if err = tx.Commit(); err != nil {
		a.dbError(w, err, "资产变更提交失败。")
		return
	}
	writeJSON(w, 200, map[string]any{"item": item})
}
