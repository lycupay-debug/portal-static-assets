package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
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

func (a *portalAPI) exportAssetsCSV(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT a.asset_id::text,a.asset_type,a.asset_name,a.asset_path,a.source_url,a.creator_or_rightsholder,a.ownership_basis,coalesce(l.spdx_id,''),a.permission_status,a.use_scope,a.distribution_status,a.review_status,a.risk_level,a.sha256,coalesce(a.file_size_bytes,0),a.published_or_active,a.notes FROM portal.compliance_assets a LEFT JOIN portal.compliance_license_catalog l ON l.license_id=a.license_id ORDER BY a.risk_level,a.asset_type,a.asset_name`)
	if err != nil {
		a.dbError(w, err, "无法导出资产台账。")
		return
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="copyright-asset-ledger.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write([]byte("\xef\xbb\xbf")); err != nil {
		log.Printf("compliance_asset_export_failed stage=bom_write")
		return
	}
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"asset_id", "asset_type", "asset_name", "asset_path", "source_url", "rightsholder", "ownership_basis", "license_spdx", "permission_status", "use_scope", "distribution_status", "review_status", "risk_level", "sha256", "file_size_bytes", "active", "notes"}); err != nil {
		log.Printf("compliance_asset_export_failed stage=header_write")
		return
	}
	for rows.Next() {
		var record []string
		var id, typ, name, path, source, rightsholder, ownership, license, permission, scope, distribution, status, risk, sha, notes string
		var size int64
		var active bool
		if err = rows.Scan(&id, &typ, &name, &path, &source, &rightsholder, &ownership, &license, &permission, &scope, &distribution, &status, &risk, &sha, &size, &active, &notes); err != nil {
			log.Printf("compliance_asset_export_failed stage=row_scan")
			return
		}
		record = []string{id, typ, name, path, source, rightsholder, ownership, license, permission, scope, distribution, status, risk, sha, strconv.FormatInt(size, 10), strconv.FormatBool(active), notes}
		if err = cw.Write(record); err != nil {
			log.Printf("compliance_asset_export_failed stage=row_write")
			return
		}
	}
	if err = rows.Err(); err != nil {
		log.Printf("compliance_asset_export_failed stage=row_iteration")
		return
	}
	cw.Flush()
	if err = cw.Error(); err != nil {
		log.Printf("compliance_asset_export_failed stage=flush")
	}
}

func (a *portalAPI) logComplianceChangeTx(ctx context.Context, tx *sql.Tx, r *http.Request, u *portalUser, action, objectType, objectID string, before map[string]any) error {
	copyState := func(state map[string]any) map[string]any {
		copy := map[string]any{}
		for key, value := range state {
			copy[key] = value
		}
		return copy
	}
	beforeJSON, err := json.Marshal(copyState(before))
	if err != nil {
		return err
	}
	queries := map[string]string{
		"asset":          "SELECT to_jsonb(x) FROM (SELECT * FROM portal.compliance_assets WHERE asset_id=$1) x",
		"license":        "SELECT to_jsonb(x) FROM (SELECT license_id,spdx_id,license_name,license_version,license_family,canonical_url,commercial_use_allowed,modification_allowed,redistribution_allowed,attribution_required,source_code_offer_required,review_status,reviewed_at,notes,updated_at FROM portal.compliance_license_catalog WHERE license_id=$1) x",
		"legal_document": "SELECT to_jsonb(x) FROM (SELECT document_id,slug,title,document_type,jurisdiction,status,version,effective_at,last_legal_review_at,missing_information,approved_by,updated_at FROM portal.compliance_legal_documents WHERE document_id=$1) x",
		"asset_evidence": "SELECT to_jsonb(x) FROM (SELECT evidence_id,asset_id,evidence_type,evidence_uri,sha256,verified,verified_by,verified_at,captured_at,notes FROM portal.compliance_asset_evidence WHERE evidence_id=$1) x",
		"portal_user":    "SELECT to_jsonb(x) FROM (SELECT user_id::text AS id,email,display_name AS displayName,is_admin AS isAdmin,is_copyright_reviewer AS isCopyrightReviewer FROM portal.portal_users WHERE user_id=$1::uuid) x",
	}
	query := queries[objectType]
	if query == "" {
		return fmt.Errorf("unsupported compliance audit object type %q", objectType)
	}
	after, err := oneJSON(ctx, tx, query, objectID)
	if err != nil {
		return fmt.Errorf("read post-change compliance state: %w", err)
	}
	after = copyState(after)
	after["actor_display_name"] = u.DisplayName
	after["request_path"] = r.URL.Path
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO portal.compliance_audit_log(actor,action,object_type,object_id,before_state,after_state) VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb)", u.DisplayName, action, objectType, objectID, string(beforeJSON), string(afterJSON)); err != nil {
		return fmt.Errorf("write compliance audit record: %w", err)
	}
	return nil
}
