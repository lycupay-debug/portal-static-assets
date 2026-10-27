package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

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
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dbError(w, err, "无法更新许可证记录。")
		return
	}
	defer tx.Rollback()
	before, err := oneJSON(r.Context(), tx, `SELECT to_jsonb(x) FROM (SELECT * FROM portal.compliance_license_catalog WHERE license_id=$1) x`, id)
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
	// #nosec G701 G202 -- every column identifier is a compile-time literal; request data is passed only as $n parameters.
	if _, err = tx.ExecContext(r.Context(), `UPDATE portal.compliance_license_catalog SET `+strings.Join(sets, ",")+fmt.Sprintf(" WHERE license_id=$%d", len(args)), args...); err != nil {
		a.dbError(w, err, "无法更新许可证记录。")
		return
	}
	if err = a.logComplianceChangeTx(r.Context(), tx, r, u, "update", "license", id, before); err != nil {
		a.dbError(w, err, "无法写入合规审计日志，许可证变更未提交。")
		return
	}
	if err = tx.Commit(); err != nil {
		a.dbError(w, err, "许可证变更提交失败。")
		return
	}
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
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dbError(w, err, "无法更新政策草稿。")
		return
	}
	defer tx.Rollback()
	before, err := oneJSON(r.Context(), tx, `SELECT to_jsonb(x) FROM (SELECT * FROM portal.compliance_legal_documents WHERE slug=$1) x`, slug)
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
	if b.MissingInformation != nil {
		raw, _ := json.Marshal(*b.MissingInformation)
		add("missing_information", string(raw))
	}
	if b.FactualAssumptions != nil {
		raw, _ := json.Marshal(*b.FactualAssumptions)
		add("factual_assumptions", string(raw))
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
		add("legal_basis", string(raw))
	}
	// JSON values are passed as parameters; PostgreSQL infers JSONB types from
	// the destination columns, so no SQL fragments are appended to user values.
	query := `UPDATE portal.compliance_legal_documents SET ` + strings.Join(sets, ",") + ` WHERE slug=$1 RETURNING document_id::text,version,body_markdown,status`
	var docID, newBody, status string
	var version int
	if err = tx.QueryRowContext(r.Context(), query, args...).Scan(&docID, &version, &newBody, &status); err != nil {
		a.dbError(w, err, "无法更新政策草稿。")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal.compliance_legal_document_versions(document_id,version,body_markdown,status,changed_by,change_note) VALUES($1,$2,$3,$4,$5,$6)`, docID, version, newBody, status, u.DisplayName, b.ChangeNote); err != nil {
		a.dbError(w, err, "无法保存政策版本历史，变更未提交。")
		return
	}
	if err = a.logComplianceChangeTx(r.Context(), tx, r, u, "update", "legal_document", docID, before); err != nil {
		a.dbError(w, err, "无法写入合规审计日志，政策变更未提交。")
		return
	}
	if err = tx.Commit(); err != nil {
		a.dbError(w, err, "政策变更提交失败。")
		return
	}
	writeJSON(w, 200, map[string]any{"id": docID, "version": version, "status": status})
}
