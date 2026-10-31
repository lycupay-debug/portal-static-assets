package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
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

func (a *portalAPI) requireComplianceAdmin(w http.ResponseWriter, r *http.Request) (*portalUser, bool) {
	u, ok := a.requireUser(w, r)
	if !ok {
		return nil, false
	}
	permission := "portal.compliance.manage"
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		permission = "portal.compliance.read"
	}
	var admin bool
	if err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM portal.portal_user_roles ur JOIN portal.role_permissions rp ON rp.role_key=ur.role_key WHERE ur.user_id=$1::uuid AND rp.permission_key=$2)`, u.ID, permission).Scan(&admin); err != nil {
		a.dbError(w, err, "无法验证版权合规管理权限。")
		return nil, false
	}
	if !admin {
		writeError(w, http.StatusForbidden, "当前账户没有版权合规管理权限。", nil)
		return nil, false
	}
	return u, true
}

func (a *portalAPI) complianceReviewerUsers(w http.ResponseWriter, r *http.Request, u *portalUser, tail string) {
	if tail == "" && r.Method == http.MethodGet {
		items, err := queryJSONRows(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT user_id::text AS id,email,display_name AS "displayName",is_admin AS "isAdmin",is_copyright_reviewer AS "isCopyrightReviewer",created_at AS "createdAt" FROM portal.portal_users ORDER BY created_at DESC) x`)
		if err != nil {
			a.dbError(w, err, "无法读取审核人员列表。")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	}
	id := strings.TrimPrefix(tail, "/")
	if !portalUUID.MatchString(id) || r.Method != http.MethodPatch {
		writeError(w, http.StatusNotFound, "审核人员记录不存在。", nil)
		return
	}
	var body struct {
		IsCopyrightReviewer *bool `json:"isCopyrightReviewer"`
	}
	if err := decodeJSON(w, r, 4*1024, &body); err != nil || body.IsCopyrightReviewer == nil {
		writeError(w, http.StatusBadRequest, "请提供 isCopyrightReviewer 布尔值。", nil)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		a.dbError(w, err, "无法开始审核人员权限事务。")
		return
	}
	defer tx.Rollback()
	var email string
	var beforeRoles []byte
	err = tx.QueryRowContext(r.Context(), `SELECT email, COALESCE((SELECT jsonb_agg(role_key ORDER BY role_key) FROM portal.portal_user_roles WHERE user_id=u.user_id),'[]'::jsonb) FROM portal.portal_users u WHERE user_id=$1::uuid FOR UPDATE`, id).Scan(&email, &beforeRoles)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "账户不存在。", nil)
		return
	}
	if err != nil {
		a.dbError(w, err, "无法读取审核人员当前角色。")
		return
	}
	if *body.IsCopyrightReviewer {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO portal.portal_user_roles(user_id,role_key,assigned_by) VALUES($1::uuid,'copyright_reviewer',$2) ON CONFLICT DO NOTHING`, id, u.Email)
	} else {
		_, err = tx.ExecContext(r.Context(), `DELETE FROM portal.portal_user_roles WHERE user_id=$1::uuid AND role_key='copyright_reviewer'`, id)
	}
	if err != nil {
		a.dbError(w, err, "无法更新内部审核专员角色。")
		return
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO portal.portal_user_roles(user_id,role_key,assigned_by) SELECT $1::uuid,'user',$2 WHERE NOT EXISTS (SELECT 1 FROM portal.portal_user_roles WHERE user_id=$1::uuid) ON CONFLICT DO NOTHING`, id, u.Email)
	if err != nil {
		a.dbError(w, err, "无法确保账户具有默认角色。")
		return
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE portal.portal_users SET is_copyright_reviewer=$1,updated_at=now() WHERE user_id=$2::uuid`, *body.IsCopyrightReviewer, id)
	if err != nil {
		a.dbError(w, err, "无法同步内部审核专员兼容字段。")
		return
	}
	afterRoles, err := tx.QueryContext(r.Context(), `SELECT role_key FROM portal.portal_user_roles WHERE user_id=$1::uuid ORDER BY role_key`, id)
	if err != nil {
		a.dbError(w, err, "无法读取更新后的角色。")
		return
	}
	var roleKeys []string
	for afterRoles.Next() {
		var roleKey string
		if err = afterRoles.Scan(&roleKey); err != nil {
			if closeErr := afterRoles.Close(); closeErr != nil {
				err = errors.Join(err, closeErr)
			}
			a.dbError(w, err, "无法读取更新后的角色。")
			return
		}
		roleKeys = append(roleKeys, roleKey)
	}
	if err = afterRoles.Err(); err != nil {
		if closeErr := afterRoles.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		a.dbError(w, err, "无法读取更新后的角色。")
		return
	}
	if err = afterRoles.Close(); err != nil {
		a.dbError(w, err, "无法关闭更新后的角色查询。")
		return
	}
	afterJSON, err := json.Marshal(roleKeys)
	if err != nil {
		a.dbError(w, err, "无法序列化更新后的角色。")
		return
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO portal.compliance_audit_log(actor,action,object_type,object_id,before_state,after_state) VALUES($1,'update_user_roles','portal_user',$2::uuid,jsonb_build_object('email',$3::text,'roles',$4::jsonb),jsonb_build_object('email',$3::text,'roles',$5::jsonb))`, u.Email, id, email, string(beforeRoles), string(afterJSON))
	if err != nil {
		a.dbError(w, err, "无法记录审核人员权限审计。")
		return
	}
	item, err := oneJSON(r.Context(), tx, `SELECT to_jsonb(x) FROM (SELECT user_id::text AS id,email,display_name AS "displayName",is_admin AS "isAdmin",is_copyright_reviewer AS "isCopyrightReviewer" FROM portal.portal_users WHERE user_id=$1::uuid) x`, id)
	if err != nil {
		a.dbError(w, err, "读取审核人员权限结果失败，事务已取消。")
		return
	}
	if err = tx.Commit(); err != nil {
		a.dbError(w, err, "审核人员权限事务提交失败。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (a *portalAPI) compliance(w http.ResponseWriter, r *http.Request) {
	u, ok := a.requireComplianceAdmin(w, r)
	if !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/compliance")
	if path == "/reviewers" || strings.HasPrefix(path, "/reviewers/") {
		var superAdmin bool
		if err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM portal.portal_users u JOIN portal.portal_user_roles ur ON ur.user_id=u.user_id WHERE u.user_id=$1::uuid AND u.is_admin=TRUE AND lower(u.email)='lycupay@gmail.com' AND ur.role_key='super_admin')`, u.ID).Scan(&superAdmin); err != nil {
			a.dbError(w, err, "无法验证超级管理员权限。")
			return
		}
		if !superAdmin {
			writeError(w, http.StatusForbidden, "只有超管可以分配内部审核专员权限。", nil)
			return
		}
		a.complianceReviewerUsers(w, r, u, strings.TrimPrefix(path, "/reviewers"))
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
	writeError(w, http.StatusMethodNotAllowed, "该合规接口不存在或不支持此请求方式。", nil)
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
