package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var complianceSlug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (a *portalAPI) publicLegal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式。", nil)
		return
	}
	if r.URL.Path == "/public/legal" {
		items, err := queryJSONRows(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT slug,title,document_type AS "documentType",jurisdiction,body_markdown AS "bodyMarkdown",version,effective_at AS "effectiveAt",updated_at AS "updatedAt" FROM portal.compliance_legal_documents WHERE status='published' ORDER BY document_type,title) x`)
		if err != nil {
			a.dbError(w, err, "暂时无法读取政策文件。")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	}
	slug := strings.TrimPrefix(r.URL.Path, "/public/legal/")
	if strings.Contains(slug, "/") || !complianceSlug.MatchString(slug) {
		writeError(w, http.StatusNotFound, "政策文件不存在。", nil)
		return
	}
	item, err := oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT slug,title,document_type AS "documentType",jurisdiction,body_markdown AS "bodyMarkdown",version,effective_at AS "effectiveAt",updated_at AS "updatedAt" FROM portal.compliance_legal_documents WHERE slug=$1 AND status='published') x`, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "政策文件不存在或尚未发布。", nil)
		} else {
			a.dbError(w, err, "暂时无法读取政策文件。")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (a *portalAPI) submitCopyrightComplaint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "不支持的请求方式。", nil)
		return
	}
	if a.rateLimited(w, r) {
		return
	}
	var enabled bool
	if err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM portal.compliance_legal_documents c WHERE c.slug='copyright-complaints' AND c.status='published' AND EXISTS (SELECT 1 FROM portal.compliance_legal_documents p WHERE p.slug='privacy-policy' AND p.status='published'))`).Scan(&enabled); err != nil {
		a.dbError(w, err, "投诉入口暂时不可用。")
		return
	}
	if !enabled {
		writeError(w, http.StatusServiceUnavailable, "版权投诉入口尚未正式启用。", nil)
		return
	}
	retentionDays, retentionErr := strconv.Atoi(strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_COMPLAINT_RETENTION_DAYS")))
	if retentionErr != nil || retentionDays < 30 || retentionDays > 3650 {
		writeError(w, http.StatusServiceUnavailable, "版权投诉入口尚未配置经确认的材料保留期限。", nil)
		return
	}
	var b struct {
		Name              string   `json:"name"`
		Email             string   `json:"email"`
		Role              string   `json:"role"`
		WorkDescription   string   `json:"workDescription"`
		ProofOfRights     string   `json:"proofOfRights"`
		InfringingURLs    []string `json:"infringingUrls"`
		RequestedAction   string   `json:"requestedAction"`
		StatementTruthful bool     `json:"statementTruthful"`
	}
	if err := decodeJSON(w, r, 32*1024, &b); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确。", nil)
		return
	}
	b.Name = strings.TrimSpace(b.Name)
	b.Email = strings.TrimSpace(b.Email)
	b.WorkDescription = strings.TrimSpace(b.WorkDescription)
	b.ProofOfRights = strings.TrimSpace(b.ProofOfRights)
	if runeLen(b.Name) < 1 || runeLen(b.Name) > 100 || !portalEmail.MatchString(b.Email) || len(b.Email) > 254 || runeLen(b.WorkDescription) < 5 || runeLen(b.WorkDescription) > 3000 || runeLen(b.ProofOfRights) > 5000 {
		writeError(w, http.StatusBadRequest, "请检查姓名、邮箱、作品说明和权利证明字段。", nil)
		return
	}
	if b.Role == "" {
		b.Role = "rightsholder"
	}
	if b.Role != "rightsholder" && b.Role != "authorized_agent" && b.Role != "other" {
		writeError(w, http.StatusBadRequest, "投诉人身份类型不正确。", nil)
		return
	}
	if b.RequestedAction == "" {
		b.RequestedAction = "review_and_remove"
	}
	if b.RequestedAction != "review_and_remove" && b.RequestedAction != "disable_access" && b.RequestedAction != "credit_or_license" && b.RequestedAction != "other" {
		writeError(w, http.StatusBadRequest, "诉求类型不正确。", nil)
		return
	}
	if len(b.InfringingURLs) < 1 || len(b.InfringingURLs) > 10 || !b.StatementTruthful {
		writeError(w, http.StatusBadRequest, "至少提供一个涉嫌侵权链接，并确认所提交信息真实。", nil)
		return
	}
	cleanURLs := make([]string, 0, len(b.InfringingURLs))
	for _, raw := range b.InfringingURLs {
		raw = strings.TrimSpace(raw)
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || len(raw) > 2048 {
			writeError(w, http.StatusBadRequest, "每个涉嫌侵权链接都必须是有效的 HTTP 或 HTTPS URL。", nil)
			return
		}
		cleanURLs = append(cleanURLs, raw)
	}
	urlsJSON, _ := json.Marshal(cleanURLs)
	var id string
	err := a.db.QueryRowContext(r.Context(), `INSERT INTO portal.compliance_copyright_complaints (complainant_name,complainant_email,complainant_role,work_description,proof_of_rights,infringing_urls,requested_action,statement_truthful,privacy_retention_until) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,now()+($9 * interval '1 day')) RETURNING complaint_id::text`, b.Name, b.Email, b.Role, b.WorkDescription, b.ProofOfRights, string(urlsJSON), b.RequestedAction, b.StatementTruthful, retentionDays).Scan(&id)
	if err != nil {
		a.dbError(w, err, "无法登记投诉，请稍后重试。")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"complaintId": id, "status": "received", "message": "投诉已登记；请保存投诉编号以便后续查询。"})
}

func (a *portalAPI) requireComplianceAdmin(w http.ResponseWriter, r *http.Request) (*portalUser, bool) {
	u, ok := a.requireUser(w, r)
	if !ok {
		return nil, false
	}
	var admin bool
	if err := a.db.QueryRowContext(r.Context(), `SELECT (is_admin OR is_copyright_reviewer) FROM portal.portal_users WHERE user_id=$1`, u.ID).Scan(&admin); err != nil {
		a.dbError(w, err, "无法验证版权合规管理权限。")
		return nil, false
	}
	if !admin {
		writeError(w, http.StatusForbidden, "当前账户没有版权合规管理权限。", nil)
		return nil, false
	}
	return u, true
}

func (a *portalAPI) compliancePreview(w http.ResponseWriter, r *http.Request, u *portalUser, tail string) {
	parts := strings.Split(strings.Trim(tail, "/"), "/")
	if len(parts) < 2 || r.Method != http.MethodGet || !portalUUID.MatchString(parts[1]) {
		writeError(w, http.StatusNotFound, "审核预览不存在。", nil)
		return
	}
	id := parts[1]
	switch parts[0] {
	case "content":
		if len(parts) != 2 {
			break
		}
		item, err := oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT c.content_id::text AS "id",c.title,c.slug,c.content_type AS "contentType",c.tags,c.parent_category AS "parentCategory",c.child_category AS "childCategory",c.summary,c.body,c.visibility,c.status,c.created_at AS "createdAt",c.updated_at AS "updatedAt",c.published_at AS "publishedAt",u.display_name AS "authorName",u.created_at AS "authorJoinedAt",(SELECT count(*)::int FROM portal.portal_content_interactions i WHERE i.content_id=c.content_id AND i.interaction_type='like') AS "likeCount",(SELECT count(*)::int FROM portal.portal_content_interactions i WHERE i.content_id=c.content_id AND i.interaction_type='favorite') AS "favoriteCount" FROM portal.portal_content c JOIN portal.portal_users u ON u.user_id=c.author_id WHERE c.content_id=$1::uuid AND c.deleted_at IS NULL) x`, id)
		if err != nil {
			writeError(w, http.StatusNotFound, "内容不存在或已删除。", nil)
			return
		}
		item["viewerLiked"] = false
		item["viewerFavorited"] = false
		writeJSON(w, http.StatusOK, map[string]any{"item": item, "previewOnly": true})
		return
	case "resource":
		if len(parts) == 3 && parts[2] == "download" {
			var stored, filename, mimeType sql.NullString
			err := a.db.QueryRowContext(r.Context(), `SELECT stored_filename,original_filename,mime_type FROM portal.teaching_resources WHERE resource_id=$1::uuid AND deleted_at IS NULL`, id).Scan(&stored, &filename, &mimeType)
			if err != nil || !stored.Valid {
				writeError(w, http.StatusNotFound, "资源不存在或已删除。", nil)
				return
			}
			f, err := os.Open(filepath.Join(a.resourceDir, filepath.Base(stored.String)))
			if err != nil {
				writeError(w, http.StatusNotFound, "资源文件缺失。", nil)
				return
			}
			defer f.Close()
			w.Header().Set("Content-Type", mimeType.String)
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(filename.String)))
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "private, no-store")
			_, _ = io.Copy(w, f)
			return
		}
		if len(parts) == 3 && parts[2] == "preview" {
			var stored, filename sql.NullString
			if err := a.db.QueryRowContext(r.Context(), `SELECT stored_filename,original_filename FROM portal.teaching_resources WHERE resource_id=$1::uuid AND deleted_at IS NULL AND lower(original_filename) LIKE '%.mp3'`, id).Scan(&stored, &filename); err != nil || !stored.Valid || !filename.Valid {
				writeError(w, http.StatusNotFound, "MP3 资源不存在或已删除。", nil)
				return
			}
			f, err := os.Open(filepath.Join(a.resourceDir, filepath.Base(stored.String)))
			if err != nil {
				writeError(w, http.StatusNotFound, "资源文件缺失。", nil)
				return
			}
			defer f.Close()
			w.Header().Set("Cache-Control", "private, no-store")
			serveMP3Preview(w, r, f, filename.String)
			return
		}
		if len(parts) != 2 {
			break
		}
		item, err := oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT resource_id::text AS "id",title,description,resource_type AS "type",grade,is_public AS "isPublic",resource_url AS "url",'/api/compliance/preview/resource/' || resource_id::text || '/download' AS "downloadUrl",original_filename AS filename,mime_type AS "mimeType",file_size AS "fileSize",created_at AS "createdAt",updated_at AS "updatedAt" FROM portal.teaching_resources WHERE resource_id=$1::uuid AND deleted_at IS NULL) x`, id)
		if err != nil {
			writeError(w, http.StatusNotFound, "资源不存在或已删除。", nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": item, "previewOnly": true})
		return
	}
	writeError(w, http.StatusNotFound, "审核预览不存在。", nil)
}

func (a *portalAPI) compliance(w http.ResponseWriter, r *http.Request) {
	u, ok := a.requireComplianceAdmin(w, r)
	if !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/compliance")
	if strings.HasPrefix(path, "/preview/") {
		a.compliancePreview(w, r, u, strings.TrimPrefix(path, "/preview/"))
		return
	}
	if strings.HasPrefix(path, "/licenses/") && r.Method == http.MethodGet {
		id := strings.TrimPrefix(path, "/licenses/")
		if !portalUUID.MatchString(id) {
			writeError(w, http.StatusNotFound, "许可证记录不存在。", nil)
			return
		}
		item, err := oneJSON(r.Context(), a.db, "SELECT to_jsonb(x) FROM (SELECT license_id::text AS \"id\",spdx_id AS \"spdxId\",license_name AS name,license_version AS version,license_family AS family,canonical_url AS \"canonicalUrl\",full_text AS \"fullText\",commercial_use_allowed AS \"commercialUseAllowed\",modification_allowed AS \"modificationAllowed\",redistribution_allowed AS \"redistributionAllowed\",attribution_required AS \"attributionRequired\",source_code_offer_required AS \"sourceCodeOfferRequired\",review_status AS \"reviewStatus\",reviewed_at AS \"reviewedAt\",notes,updated_at AS \"updatedAt\" FROM portal.compliance_license_catalog WHERE license_id=$1::uuid) x", id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "许可证记录不存在。", nil)
			} else {
				a.dbError(w, err, "无法读取许可证原文。")
			}
			return
		}
		linked, err := queryJSONRows(r.Context(), a.db, "SELECT to_jsonb(x) FROM (SELECT asset_id::text AS \"id\",asset_type AS \"assetType\",asset_name AS name,asset_path AS path,source_url AS \"sourceUrl\",permission_status AS \"permissionStatus\",review_status AS \"reviewStatus\",risk_level AS \"riskLevel\",sha256 FROM portal.compliance_assets WHERE license_id=$1::uuid ORDER BY asset_type,asset_name) x", id)
		if err != nil {
			a.dbError(w, err, "无法读取关联资产。")
			return
		}
		item["linkedAssets"] = linked
		writeJSON(w, http.StatusOK, map[string]any{"item": item})
		return
	}
	switch path {
	case "/overview":
		if r.Method != http.MethodGet {
			break
		}
		var result = map[string]any{}
		queries := map[string]string{
			"assetCount":             `SELECT count(*) FROM portal.compliance_assets`,
			"pendingAssetCount":      `SELECT count(*) FROM portal.compliance_assets WHERE review_status IN ('pending','in_review')`,
			"highRiskCount":          `SELECT count(*) FROM portal.compliance_assets WHERE risk_level IN ('high','critical')`,
			"unknownPermissionCount": `SELECT count(*) FROM portal.compliance_assets WHERE permission_status='unknown'`,
			"licenseCount":           `SELECT count(*) FROM portal.compliance_license_catalog`,
			"unreviewedLicenseCount": `SELECT count(*) FROM portal.compliance_license_catalog WHERE review_status='unreviewed'`,
			"openComplaintCount":     `SELECT count(*) FROM portal.compliance_copyright_complaints WHERE status NOT IN ('closed','rejected')`,
			"draftDocumentCount":     `SELECT count(*) FROM portal.compliance_legal_documents WHERE status IN ('draft','in_review')`,
		}
		for key, query := range queries {
			var n int
			if err := a.db.QueryRowContext(r.Context(), query).Scan(&n); err != nil {
				a.dbError(w, err, "无法读取合规统计。")
				return
			}
			result[key] = n
		}
		assets, err := queryJSONRows(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT review_status AS status,count(*)::int AS count FROM portal.compliance_assets GROUP BY review_status ORDER BY review_status) x`)
		if err != nil {
			a.dbError(w, err, "无法读取审核状态统计。")
			return
		}
		risk, err := queryJSONRows(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT risk_level AS level,count(*)::int AS count FROM portal.compliance_assets GROUP BY risk_level ORDER BY risk_level) x`)
		if err != nil {
			a.dbError(w, err, "无法读取风险统计。")
			return
		}
		result["assetsByStatus"], result["assetsByRisk"] = assets, risk
		writeJSON(w, http.StatusOK, result)
		return
	case "/assets":
		if r.Method != http.MethodGet {
			break
		}
		a.listComplianceAssets(w, r)
		return
	case "/licenses":
		if r.Method == http.MethodGet {
			items, err := queryJSONRows(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT license_id::text AS id,spdx_id AS "spdxId",license_name AS name,license_version AS version,license_family AS family,canonical_url AS "canonicalUrl",commercial_use_allowed AS "commercialUseAllowed",modification_allowed AS "modificationAllowed",redistribution_allowed AS "redistributionAllowed",attribution_required AS "attributionRequired",source_code_offer_required AS "sourceCodeOfferRequired",review_status AS "reviewStatus",reviewed_at AS "reviewedAt",notes,CASE WHEN full_text='' THEN false ELSE true END AS "licenseTextCaptured",updated_at AS "updatedAt" FROM portal.compliance_license_catalog ORDER BY review_status,license_name,license_version) x`)
			if err != nil {
				a.dbError(w, err, "无法读取许可证目录。")
				return
			}
			writeJSON(w, 200, map[string]any{"items": items})
			return
		}
	case "/documents":
		if r.Method == http.MethodGet {
			items, err := queryJSONRows(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT document_id::text AS id,slug,title,document_type AS "documentType",jurisdiction,body_markdown AS "bodyMarkdown",status,version,effective_at AS "effectiveAt",last_legal_review_at AS "lastLegalReviewAt",legal_basis AS "legalBasis",factual_assumptions AS "factualAssumptions",missing_information AS "missingInformation",approved_by AS "approvedBy",updated_at AS "updatedAt" FROM portal.compliance_legal_documents ORDER BY document_type,title) x`)
			if err != nil {
				a.dbError(w, err, "无法读取政策草稿。")
				return
			}
			writeJSON(w, 200, map[string]any{"items": items})
			return
		}
	case "/complaints":
		if !u.IsAdmin {
			writeError(w, http.StatusForbidden, "版权审核专员无权查看投诉人的个人信息。", nil)
			return
		}
		if r.Method == http.MethodGet {
			items, err := queryJSONRows(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT complaint_id::text AS id,complainant_name AS name,complainant_email AS email,complainant_role AS role,work_description AS "workDescription",proof_of_rights AS "proofOfRights",infringing_urls AS "infringingUrls",requested_action AS "requestedAction",statement_truthful AS "statementTruthful",status,resolution_notes AS "resolutionNotes",assigned_to AS "assignedTo",received_at AS "receivedAt",updated_at AS "updatedAt",resolved_at AS "resolvedAt" FROM portal.compliance_copyright_complaints ORDER BY received_at DESC LIMIT 500) x`)
			if err != nil {
				a.dbError(w, err, "无法读取版权投诉。")
				return
			}
			writeJSON(w, 200, map[string]any{"items": items})
			return
		}
	case "/audit":
		if r.Method == http.MethodGet {
			items, err := queryJSONRows(r.Context(), a.db, "SELECT to_jsonb(x) FROM (SELECT audit_id::text AS id,actor,action,object_type AS \"objectType\",object_id::text AS \"objectId\",before_state AS \"beforeState\",after_state AS \"afterState\",created_at AS \"createdAt\" FROM portal.compliance_audit_log ORDER BY created_at DESC LIMIT 200) x")
			if err != nil {
				a.dbError(w, err, "无法读取合规操作日志。")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"items": items})
			return
		}
	case "/scans":
		if r.Method == http.MethodGet {
			items, err := queryJSONRows(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT scan_id::text AS id,scan_type AS "scanType",scan_status AS status,started_at AS "startedAt",completed_at AS "completedAt",source_revision AS "sourceRevision",assets_seen AS "assetsSeen",assets_created AS "assetsCreated",assets_updated AS "assetsUpdated",findings_count AS "findingsCount",report_path AS "reportPath",summary,error_message AS "errorMessage" FROM portal.compliance_scan_runs ORDER BY started_at DESC LIMIT 100) x`)
			if err != nil {
				a.dbError(w, err, "无法读取扫描记录。")
				return
			}
			writeJSON(w, 200, map[string]any{"items": items})
			return
		}
	case "/export/assets.csv":
		if r.Method == http.MethodGet {
			a.exportAssetsCSV(w, r)
			return
		}
	}
	if strings.HasPrefix(path, "/assets/") {
		a.complianceAssetByID(w, r, u, strings.TrimPrefix(path, "/assets/"))
		return
	}
	if strings.HasPrefix(path, "/licenses/") {
		a.complianceLicenseByID(w, r, u, strings.TrimPrefix(path, "/licenses/"))
		return
	}
	if strings.HasPrefix(path, "/documents/") {
		a.complianceDocumentBySlug(w, r, u, strings.TrimPrefix(path, "/documents/"))
		return
	}
	if strings.HasPrefix(path, "/complaints/") {
		a.complianceComplaintByID(w, r, u, strings.TrimPrefix(path, "/complaints/"))
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "该合规接口不存在或不支持此请求方式。", nil)
}

func (a *portalAPI) listComplianceAssets(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	typ := q.Get("type")
	status := q.Get("status")
	risk := q.Get("risk")
	search := strings.TrimSpace(q.Get("q"))
	if typ != "" && !oneOf(typ, "image", "icon", "font", "audio", "video", "article", "education_resource", "software_dependency", "source_file", "policy_document", "external_link", "other") {
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
	base := ` FROM portal.compliance_assets WHERE ($1='' OR asset_type=$1) AND ($2='' OR review_status=$2) AND ($3='' OR risk_level=$3) AND ($4='' OR asset_name ILIKE '%'||$4||'%' OR source_url ILIKE '%'||$4||'%' OR asset_path ILIKE '%'||$4||'%')`
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
	before, err := oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT * FROM portal.compliance_assets WHERE asset_id=$1) x`, id)
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
	if _, err = a.db.ExecContext(r.Context(), `UPDATE portal.compliance_assets SET `+strings.Join(sets, ",")+fmt.Sprintf(" WHERE asset_id=$%d", len(args)), args...); err != nil {
		a.dbError(w, err, "无法更新资产台账。")
		return
	}
	a.logComplianceChange(r, u, "update", "asset", id, before)
	item, err := oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT * FROM portal.compliance_assets WHERE asset_id=$1) x`, id)
	if err != nil {
		a.dbError(w, err, "资产已更新，但读取结果失败。")
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
	if b.Verified && strings.TrimSpace(b.VerifiedBy) == "" {
		writeError(w, 400, "标记为已核验时必须填写核验人。", nil)
		return
	}
	var assetExists bool
	if err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM portal.compliance_assets WHERE asset_id=$1)`, id).Scan(&assetExists); err != nil || !assetExists {
		writeError(w, 404, "资产不存在。", nil)
		return
	}
	var evidenceID string
	var verifiedAt any
	if b.Verified {
		verifiedAt = time.Now().UTC()
	}
	err := a.db.QueryRowContext(r.Context(), `INSERT INTO portal.compliance_asset_evidence(asset_id,evidence_type,evidence_uri,evidence_text,sha256,verified,verified_by,verified_at,notes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING evidence_id::text`, id, b.Type, strings.TrimSpace(b.URI), b.Text, b.SHA256, b.Verified, strings.TrimSpace(b.VerifiedBy), verifiedAt, b.Notes).Scan(&evidenceID)
	if err != nil {
		a.dbError(w, err, "无法保存证据。")
		return
	}
	a.logComplianceChange(r, u, "create", "asset_evidence", evidenceID, nil)
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
	if !oneOf(b.Type, "copyright", "license", "privacy", "content", "distribution", "legal_basis", "recheck") || !oneOf(b.Decision, "approved", "approved_with_conditions", "pending_evidence", "replace", "remove", "blocked", "escalate") || !oneOf(b.RiskLevel, "low", "medium", "high", "critical", "unknown") || runeLen(b.Findings) > 10000 || len(b.RequiredActions) > 30 {
		writeError(w, 400, "审核字段不正确。", nil)
		return
	}
	var exists bool
	if err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM portal.compliance_assets WHERE asset_id=$1)`, id).Scan(&exists); err != nil || !exists {
		writeError(w, 404, "资产不存在。", nil)
		return
	}
	actions, _ := json.Marshal(b.RequiredActions)
	var reviewID string
	err := a.db.QueryRowContext(r.Context(), `INSERT INTO portal.compliance_asset_reviews(asset_id,reviewer,review_type,decision,risk_level,findings,required_actions,follow_up_at) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8) RETURNING review_id::text`, id, u.DisplayName, b.Type, b.Decision, b.RiskLevel, b.Findings, string(actions), b.FollowUpAt).Scan(&reviewID)
	if err != nil {
		a.dbError(w, err, "无法保存审核记录。")
		return
	}
	status := map[string]string{"approved": "approved", "approved_with_conditions": "approved_with_conditions", "pending_evidence": "pending", "replace": "replace_or_remove", "remove": "blocked", "blocked": "blocked", "escalate": "in_review"}[b.Decision]
	before, _ := oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT * FROM portal.compliance_assets WHERE asset_id=$1) x`, id)
	if _, err = a.db.ExecContext(r.Context(), `UPDATE portal.compliance_assets SET review_status=$1,risk_level=$2,updated_at=now() WHERE asset_id=$3`, status, b.RiskLevel, id); err != nil {
		a.dbError(w, err, "审核已登记，但资产状态更新失败。")
		return
	}
	a.logComplianceChange(r, u, "review", "asset", id, before)
	writeJSON(w, 201, map[string]any{"reviewId": reviewID, "reviewStatus": status, "note": "审核记录已保存；本操作不会自动删除、下架或修改公开内容。"})
}

func (a *portalAPI) complianceLicenseByID(w http.ResponseWriter, r *http.Request, u *portalUser, tail string) {
	id := strings.Trim(tail, "/")
	if !portalUUID.MatchString(id) {
		writeError(w, 404, "许可证记录不存在。", nil)
		return
	}
	if r.Method != http.MethodPatch && r.Method != http.MethodPut {
		writeError(w, 405, "不支持的请求方式。", nil)
		return
	}
	var b struct {
		ReviewStatus            *string `json:"reviewStatus"`
		Notes                   *string `json:"notes"`
		Family                  *string `json:"family"`
		CommercialUseAllowed    *bool   `json:"commercialUseAllowed"`
		ModificationAllowed     *bool   `json:"modificationAllowed"`
		RedistributionAllowed   *bool   `json:"redistributionAllowed"`
		AttributionRequired     *bool   `json:"attributionRequired"`
		SourceCodeOfferRequired *bool   `json:"sourceCodeOfferRequired"`
	}
	if err := decodeJSON(w, r, 32*1024, &b); err != nil {
		writeError(w, 400, "请求格式不正确。", nil)
		return
	}
	if b.ReviewStatus != nil && !oneOf(*b.ReviewStatus, "unreviewed", "reviewed", "restricted", "needs_counsel") {
		writeError(w, 400, "许可证审核状态不正确。", nil)
		return
	}
	if b.Family != nil && !oneOf(*b.Family, "permissive", "copyleft", "weak_copyleft", "proprietary", "public_domain", "custom", "other") {
		writeError(w, 400, "许可证类型不正确。", nil)
		return
	}
	before, err := oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT * FROM portal.compliance_license_catalog WHERE license_id=$1) x`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, 404, "许可证记录不存在。", nil)
		} else {
			a.dbError(w, err, "无法读取许可证记录。")
		}
		return
	}
	sets := []string{"updated_at=now()"}
	args := []any{}
	add := func(c string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s=$%d", c, len(args)))
	}
	if b.ReviewStatus != nil {
		add("review_status", *b.ReviewStatus)
		if *b.ReviewStatus == "reviewed" {
			sets = append(sets, "reviewed_at=now()")
		}
	}
	if b.Notes != nil {
		add("notes", *b.Notes)
	}
	if b.Family != nil {
		add("license_family", *b.Family)
	}
	if b.CommercialUseAllowed != nil {
		add("commercial_use_allowed", *b.CommercialUseAllowed)
	}
	if b.ModificationAllowed != nil {
		add("modification_allowed", *b.ModificationAllowed)
	}
	if b.RedistributionAllowed != nil {
		add("redistribution_allowed", *b.RedistributionAllowed)
	}
	if b.AttributionRequired != nil {
		add("attribution_required", *b.AttributionRequired)
	}
	if b.SourceCodeOfferRequired != nil {
		add("source_code_offer_required", *b.SourceCodeOfferRequired)
	}
	if len(args) == 0 {
		writeError(w, 400, "没有可更新的字段。", nil)
		return
	}
	args = append(args, id)
	if _, err = a.db.ExecContext(r.Context(), `UPDATE portal.compliance_license_catalog SET `+strings.Join(sets, ",")+fmt.Sprintf(" WHERE license_id=$%d", len(args)), args...); err != nil {
		a.dbError(w, err, "无法更新许可证记录。")
		return
	}
	a.logComplianceChange(r, u, "update", "license", id, before)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *portalAPI) complianceDocumentBySlug(w http.ResponseWriter, r *http.Request, u *portalUser, slug string) {
	slug = strings.Trim(slug, "/")
	if strings.Contains(slug, "/") || !complianceSlug.MatchString(slug) {
		writeError(w, 404, "政策文档不存在。", nil)
		return
	}
	if r.Method != http.MethodPatch && r.Method != http.MethodPut {
		writeError(w, 405, "不支持的请求方式。", nil)
		return
	}
	var b struct {
		BodyMarkdown       *string              `json:"bodyMarkdown"`
		Status             *string              `json:"status"`
		MissingInformation *[]string            `json:"missingInformation"`
		FactualAssumptions *[]string            `json:"factualAssumptions"`
		ApprovedBy         *string              `json:"approvedBy"`
		LegalReviewAt      *time.Time           `json:"legalReviewAt"`
		EffectiveAt        *time.Time           `json:"effectiveAt"`
		LegalBasis         *[]map[string]string `json:"legalBasis"`
		ChangeNote         string               `json:"changeNote"`
	}
	if err := decodeJSON(w, r, 256*1024, &b); err != nil {
		writeError(w, 400, "请求格式不正确。", nil)
		return
	}
	before, err := oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT * FROM portal.compliance_legal_documents WHERE slug=$1) x`, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, 404, "政策文档不存在。", nil)
		} else {
			a.dbError(w, err, "无法读取政策文档。")
		}
		return
	}
	currentStatus, _ := before["status"].(string)
	newStatus := currentStatus
	if b.Status != nil {
		newStatus = *b.Status
	}
	if !oneOf(newStatus, "draft", "in_review", "published", "archived") {
		writeError(w, 400, "政策状态不正确。", nil)
		return
	}
	body, _ := before["body_markdown"].(string)
	if b.BodyMarkdown != nil {
		body = *b.BodyMarkdown
	}
	missing := []string{}
	if raw, ok := before["missing_information"].([]any); ok {
		for _, v := range raw {
			if s, ok := v.(string); ok {
				missing = append(missing, s)
			}
		}
	}
	if b.MissingInformation != nil {
		missing = *b.MissingInformation
	}
	approved, _ := before["approved_by"].(string)
	if b.ApprovedBy != nil {
		approved = *b.ApprovedBy
	}
	var reviewAt *time.Time
	if s, ok := before["last_legal_review_at"].(string); ok && s != "" {
		if parsed, e := time.Parse(time.RFC3339Nano, s); e == nil {
			reviewAt = &parsed
		}
	}
	if b.LegalReviewAt != nil {
		reviewAt = b.LegalReviewAt
	}
	if newStatus == "published" {
		if len(missing) > 0 || strings.Contains(body, "【") || strings.Contains(body, "】") || strings.TrimSpace(approved) == "" || reviewAt == nil {
			writeError(w, 409, "政策尚不能发布：请先补齐缺失事实、移除占位符、记录法律审核时间并填写审核人。", nil)
			return
		}
	}
	args := []any{slug, newStatus}
	sets := []string{"status=$2", "version=version+1", "updated_at=now()"}
	idx := 2
	add := func(col string, v any) {
		idx++
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s=$%d", col, idx))
	}
	if b.BodyMarkdown != nil {
		add("body_markdown", *b.BodyMarkdown)
	}
	if b.Status != nil { /* status already bound */
	}
	if b.MissingInformation != nil {
		raw, _ := json.Marshal(*b.MissingInformation)
		add("missing_information", string(raw)+"::jsonb")
	}
	if b.FactualAssumptions != nil {
		raw, _ := json.Marshal(*b.FactualAssumptions)
		add("factual_assumptions", string(raw)+"::jsonb")
	}
	if b.ApprovedBy != nil {
		add("approved_by", strings.TrimSpace(*b.ApprovedBy))
	}
	if b.LegalReviewAt != nil {
		add("last_legal_review_at", *b.LegalReviewAt)
	}
	if b.EffectiveAt != nil {
		add("effective_at", *b.EffectiveAt)
	}
	if b.LegalBasis != nil {
		raw, _ := json.Marshal(*b.LegalBasis)
		add("legal_basis", string(raw)+"::jsonb")
	}
	// JSONB values above are cast as SQL text parameters by the explicit placeholder cast below.
	for i := range args {
		_ = i
	}
	query := `UPDATE portal.compliance_legal_documents SET ` + strings.Join(sets, ",") + ` WHERE slug=$1 RETURNING document_id::text,version,body_markdown,status`
	// The JSONB placeholders are passed as text and cast in a follow-up update to keep input strictly parameterized.
	query = strings.ReplaceAll(query, "missing_information=$", "missing_information=$")
	var docID, newBody, status string
	var version int
	// Normalize json strings to []byte-compatible values; pgx accepts []byte for JSONB.
	for i := 1; i < len(args); i++ {
		if s, ok := args[i].(string); ok && (strings.HasSuffix(s, "::jsonb")) {
			args[i] = strings.TrimSuffix(s, "::jsonb")
		}
	}
	if err = a.db.QueryRowContext(r.Context(), query, args...).Scan(&docID, &version, &newBody, &status); err != nil {
		a.dbError(w, err, "无法更新政策草稿。")
		return
	}
	if _, err = a.db.ExecContext(r.Context(), `INSERT INTO portal.compliance_legal_document_versions(document_id,version,body_markdown,status,changed_by,change_note) VALUES($1,$2,$3,$4,$5,$6)`, docID, version, newBody, status, u.DisplayName, b.ChangeNote); err != nil {
		a.dbError(w, err, "政策已更新，但版本历史写入失败。")
		return
	}
	a.logComplianceChange(r, u, "update", "legal_document", docID, before)
	writeJSON(w, 200, map[string]any{"id": docID, "version": version, "status": status})
}

func (a *portalAPI) complianceComplaintByID(w http.ResponseWriter, r *http.Request, u *portalUser, tail string) {
	id := strings.Trim(tail, "/")
	if !portalUUID.MatchString(id) {
		writeError(w, 404, "投诉记录不存在。", nil)
		return
	}
	if r.Method != http.MethodPatch && r.Method != http.MethodPut {
		writeError(w, 405, "不支持的请求方式。", nil)
		return
	}
	var b struct {
		Status          *string `json:"status"`
		ResolutionNotes *string `json:"resolutionNotes"`
		AssignedTo      *string `json:"assignedTo"`
	}
	if err := decodeJSON(w, r, 32*1024, &b); err != nil {
		writeError(w, 400, "请求格式不正确。", nil)
		return
	}
	if b.Status != nil && !oneOf(*b.Status, "received", "under_review", "need_more_information", "action_taken", "rejected", "closed") {
		writeError(w, 400, "投诉状态不正确。", nil)
		return
	}
	before, err := oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT * FROM portal.compliance_copyright_complaints WHERE complaint_id=$1) x`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, 404, "投诉记录不存在。", nil)
		} else {
			a.dbError(w, err, "无法读取投诉记录。")
		}
		return
	}
	sets := []string{"updated_at=now()"}
	args := []any{}
	add := func(c string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s=$%d", c, len(args)))
	}
	if b.Status != nil {
		add("status", *b.Status)
		if *b.Status == "closed" || *b.Status == "rejected" || *b.Status == "action_taken" {
			sets = append(sets, "resolved_at=now()")
		}
	}
	if b.ResolutionNotes != nil {
		add("resolution_notes", *b.ResolutionNotes)
	}
	if b.AssignedTo != nil {
		add("assigned_to", *b.AssignedTo)
	}
	if len(args) == 0 {
		writeError(w, 400, "没有可更新的字段。", nil)
		return
	}
	args = append(args, id)
	if _, err = a.db.ExecContext(r.Context(), `UPDATE portal.compliance_copyright_complaints SET `+strings.Join(sets, ",")+fmt.Sprintf(" WHERE complaint_id=$%d", len(args)), args...); err != nil {
		a.dbError(w, err, "无法更新投诉记录。")
		return
	}
	a.logComplianceChange(r, u, "update", "copyright_complaint", id, before)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *portalAPI) exportAssetsCSV(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT asset_id::text,asset_type,asset_name,asset_path,source_url,creator_or_rightsholder,ownership_basis,coalesce(l.spdx_id,''),permission_status,use_scope,distribution_status,review_status,risk_level,sha256,coalesce(file_size_bytes,0),published_or_active,notes FROM portal.compliance_assets a LEFT JOIN portal.compliance_license_catalog l ON l.license_id=a.license_id ORDER BY risk_level,asset_type,asset_name`)
	if err != nil {
		a.dbError(w, err, "无法导出资产台账。")
		return
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="copyright-asset-ledger.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte("\xef\xbb\xbf"))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"asset_id", "asset_type", "asset_name", "asset_path", "source_url", "rightsholder", "ownership_basis", "license_spdx", "permission_status", "use_scope", "distribution_status", "review_status", "risk_level", "sha256", "file_size_bytes", "active", "notes"})
	for rows.Next() {
		var record []string
		var id, typ, name, path, source, rightsholder, ownership, license, permission, scope, distribution, status, risk, sha, notes string
		var size int64
		var active bool
		if err = rows.Scan(&id, &typ, &name, &path, &source, &rightsholder, &ownership, &license, &permission, &scope, &distribution, &status, &risk, &sha, &size, &active, &notes); err != nil {
			return
		}
		record = []string{id, typ, name, path, source, rightsholder, ownership, license, permission, scope, distribution, status, risk, sha, strconv.FormatInt(size, 10), strconv.FormatBool(active), notes}
		_ = cw.Write(record)
	}
	cw.Flush()
}

func (a *portalAPI) logComplianceChange(r *http.Request, u *portalUser, action, objectType, objectID string, before map[string]any) {
	redact := func(state map[string]any) map[string]any {
		copyState := map[string]any{}
		for key, value := range state {
			if objectType == "copyright_complaint" && (key == "complainant_name" || key == "complainant_email" || key == "proof_of_rights" || key == "infringing_urls") {
				continue
			}
			copyState[key] = value
		}
		return copyState
	}
	beforeJSON, _ := json.Marshal(redact(before))
	queries := map[string]string{
		"asset":               "SELECT to_jsonb(x) FROM (SELECT * FROM portal.compliance_assets WHERE asset_id=$1) x",
		"license":             "SELECT to_jsonb(x) FROM (SELECT license_id,spdx_id,license_name,license_version,license_family,canonical_url,commercial_use_allowed,modification_allowed,redistribution_allowed,attribution_required,source_code_offer_required,review_status,reviewed_at,notes,updated_at FROM portal.compliance_license_catalog WHERE license_id=$1) x",
		"legal_document":      "SELECT to_jsonb(x) FROM (SELECT document_id,slug,title,document_type,jurisdiction,status,version,effective_at,last_legal_review_at,missing_information,approved_by,updated_at FROM portal.compliance_legal_documents WHERE document_id=$1) x",
		"copyright_complaint": "SELECT to_jsonb(x) FROM (SELECT complaint_id,status,requested_action,resolution_notes,assigned_to,received_at,updated_at,resolved_at FROM portal.compliance_copyright_complaints WHERE complaint_id=$1) x",
		"asset_evidence":      "SELECT to_jsonb(x) FROM (SELECT evidence_id,asset_id,evidence_type,evidence_uri,sha256,verified,verified_by,verified_at,captured_at,notes FROM portal.compliance_asset_evidence WHERE evidence_id=$1) x",
	}
	after := map[string]any{}
	var readErr error
	if query := queries[objectType]; query != "" {
		after, readErr = oneJSON(r.Context(), a.db, query, objectID)
	}
	if readErr != nil || after == nil {
		after = map[string]any{"request_path": r.URL.Path, "state_read_error": readErr != nil}
	}
	after = redact(after)
	after["actor_display_name"] = u.DisplayName
	after["request_path"] = r.URL.Path
	afterJSON, _ := json.Marshal(after)
	_, _ = a.db.ExecContext(r.Context(), "INSERT INTO portal.compliance_audit_log(actor,action,object_type,object_id,before_state,after_state) VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb)", u.DisplayName, action, objectType, objectID, string(beforeJSON), string(afterJSON))
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
