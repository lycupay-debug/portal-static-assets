package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type publisherRightsSubmission struct {
	OwnershipBasis    string
	Rightsholder      string
	SourceURL         string
	UseScope          string
	LicenseName       string
	LicenseURL        string
	PermissionExpiry  string
	EvidenceType      string
	EvidenceURI       string
	EvidenceText      string
	EvidenceFilename  string
	EvidenceData      string
	StatementAccepted bool
}

func validOptionalHTTPURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	u, err := url.ParseRequestURI(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func (a *portalAPI) submitPublisherRights(w http.ResponseWriter, r *http.Request, u *portalUser, kind, id string) {
	var b publisherRightsSubmission
	if err := decodeJSON(w, r, 15*1024*1024, &b); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确。", nil)
		return
	}
	b.OwnershipBasis = strings.TrimSpace(b.OwnershipBasis)
	b.Rightsholder = strings.TrimSpace(b.Rightsholder)
	b.SourceURL = strings.TrimSpace(b.SourceURL)
	b.UseScope = strings.TrimSpace(b.UseScope)
	b.LicenseName = strings.TrimSpace(b.LicenseName)
	b.LicenseURL = strings.TrimSpace(b.LicenseURL)
	b.PermissionExpiry = strings.TrimSpace(b.PermissionExpiry)
	b.EvidenceURI = strings.TrimSpace(b.EvidenceURI)
	b.EvidenceText = strings.TrimSpace(b.EvidenceText)
	b.EvidenceFilename = filepath.Base(strings.TrimSpace(b.EvidenceFilename))
	if len(b.EvidenceData) > 14*1024*1024 || (b.EvidenceData != "" && b.EvidenceFilename == "") {
		writeError(w, http.StatusBadRequest, "证据文件信息不正确或文件过大。", nil)
		return
	}
	var evidenceBytes []byte
	if b.EvidenceData != "" {
		var decodeErr error
		evidenceBytes, decodeErr = base64.StdEncoding.DecodeString(b.EvidenceData)
		if decodeErr != nil || len(evidenceBytes) == 0 || len(evidenceBytes) > 10*1024*1024 {
			writeError(w, http.StatusBadRequest, "证据文件编码无效或超过 10 MB。", nil)
			return
		}
		ext := strings.ToLower(filepath.Ext(b.EvidenceFilename))
		if !oneOf(ext, ".pdf", ".png", ".jpg", ".jpeg", ".webp", ".txt", ".doc", ".docx") {
			writeError(w, http.StatusBadRequest, "证据文件类型不受支持。", nil)
			return
		}
	}
	if !b.StatementAccepted || !oneOf(b.OwnershipBasis, "original", "commissioned", "licensed", "open_license", "public_domain", "third_party_permission") ||
		!oneOf(b.UseScope, "commercial_web", "internal_only", "redistribution", "modification", "multiple") ||
		len([]rune(b.Rightsholder)) > 500 || len([]rune(b.SourceURL)) > 2048 || len([]rune(b.LicenseName)) > 500 ||
		len([]rune(b.LicenseURL)) > 2048 || len([]rune(b.EvidenceURI)) > 2048 || len([]rune(b.EvidenceText)) > 50000 ||
		!validOptionalHTTPURL(b.SourceURL) || !validOptionalHTTPURL(b.LicenseURL) || !validOptionalHTTPURL(b.EvidenceURI) {
		writeError(w, http.StatusBadRequest, "请填写有效的权利类型、使用范围、证据和真实性声明。", nil)
		return
	}
	if (b.OwnershipBasis == "licensed" || b.OwnershipBasis == "third_party_permission" || b.OwnershipBasis == "open_license" || b.OwnershipBasis == "commissioned" || b.OwnershipBasis == "public_domain") && b.EvidenceURI == "" && b.EvidenceText == "" && len(evidenceBytes) == 0 {
		writeError(w, http.StatusBadRequest, "第三方授权或开放许可必须提供授权材料、许可文本或可核验的证据链接。", nil)
		return
	}
	if b.OwnershipBasis == "open_license" && b.LicenseName == "" {
		writeError(w, http.StatusBadRequest, "开放许可必须填写许可证名称或版本。", nil)
		return
	}
	if b.PermissionExpiry != "" {
		if _, err := time.Parse("2006-01-02", b.PermissionExpiry); err != nil {
			writeError(w, http.StatusBadRequest, "授权有效期必须使用 YYYY-MM-DD 格式。", nil)
			return
		}
	}
	assetPath := ""
	var expectedHash string
	if kind == "content" {
		var body string
		err := a.db.QueryRowContext(r.Context(), "SELECT body FROM portal.portal_content WHERE content_id=$1::uuid AND author_id=$2::uuid AND deleted_at IS NULL", id, u.ID).Scan(&body)
		if err != nil {
			if err == sql.ErrNoRows {
				writeError(w, http.StatusNotFound, "内容不存在或无权提交版权材料。", nil)
			} else {
				a.dbError(w, err, "读取内容失败。")
			}
			return
		}
		assetPath = "db://portal.portal_content/" + id
		expectedHash = contentBodyHash(body)
	} else if kind == "resource" {
		var stored, title, description, resourceType, resourceURL string
		err := a.db.QueryRowContext(r.Context(), "SELECT COALESCE(stored_filename,''),title,description,resource_type,COALESCE(resource_url,'') FROM portal.teaching_resources WHERE resource_id=$1::uuid AND owner_id=$2::uuid AND deleted_at IS NULL", id, u.ID).Scan(&stored, &title, &description, &resourceType, &resourceURL)
		if err != nil {
			if err == sql.ErrNoRows {
				writeError(w, http.StatusNotFound, "资源不存在或无权提交版权材料。", nil)
			} else {
				a.dbError(w, err, "读取资源失败。")
			}
			return
		}
		var hashErr error
		expectedHash, hashErr = a.resourceAssetHash(stored, title, description, resourceURL)
		if hashErr != nil {
			a.dbError(w, hashErr, "计算资源版本哈希失败。")
			return
		}
		assetPath = "db://portal.teaching_resources/" + id
	} else {
		writeError(w, http.StatusNotFound, "未知的内容类型。", nil)
		return
	}

	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dbError(w, err, "无法开始版权材料提交。")
		return
	}
	defer tx.Rollback()
	var storedEvidencePath string
	transactionCommitted := false
	defer func() {
		if !transactionCommitted && storedEvidencePath != "" {
			_ = os.Remove(storedEvidencePath)
		}
	}()
	var assetID string
	err = tx.QueryRowContext(r.Context(), "SELECT asset_id::text FROM portal.compliance_assets WHERE asset_path=$1 AND sha256=$2 AND COALESCE((metadata->>'superseded')::boolean,FALSE)=FALSE ORDER BY updated_at DESC LIMIT 1 FOR UPDATE", assetPath, expectedHash).Scan(&assetID)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusConflict, "当前版本尚未建立版权资产记录，请先保存内容并刷新后再提交。", nil)
		} else {
			a.dbError(w, err, "读取当前版权资产失败。")
		}
		return
	}
	claim := map[string]any{
		"ownershipBasis": b.OwnershipBasis, "rightsholder": b.Rightsholder, "sourceUrl": b.SourceURL,
		"useScope": b.UseScope, "licenseName": b.LicenseName, "licenseUrl": b.LicenseURL,
		"permissionExpiry": b.PermissionExpiry, "statementAccepted": true,
		"submittedBy": u.ID, "submittedAt": time.Now().UTC().Format(time.RFC3339Nano), "reviewStatus": "pending",
	}
	claimJSON, _ := json.Marshal(claim)
	_, err = tx.ExecContext(r.Context(), "UPDATE portal.compliance_assets SET ownership_basis=$2,creator_or_rightsholder=CASE WHEN $3='' THEN creator_or_rightsholder ELSE $3 END,source_url=CASE WHEN $4='' THEN source_url ELSE $4 END,use_scope=$5,permission_status='unknown',review_status='pending',risk_level=CASE WHEN $2 IN ('licensed','third_party_permission','open_license') THEN 'high' ELSE risk_level END,published_or_active=FALSE,distribution_status='not_distributed',metadata=metadata || jsonb_build_object('publisher_rights_submission',$6::jsonb),notes=CASE WHEN notes='' THEN 'Publisher submitted rights information; awaiting independent review.' ELSE notes || ' | Publisher submitted rights information; awaiting independent review.' END,updated_at=now() WHERE asset_id=$1::uuid", assetID, b.OwnershipBasis, b.Rightsholder, b.SourceURL, b.UseScope, string(claimJSON))
	if err != nil {
		a.dbError(w, err, "保存权利声明失败。")
		return
	}
	evidenceType := strings.TrimSpace(b.EvidenceType)
	if evidenceType == "" {
		switch b.OwnershipBasis {
		case "original":
			evidenceType = "author_declaration"
		case "commissioned":
			evidenceType = "contract"
		case "licensed", "third_party_permission":
			evidenceType = "permission_email"
		case "open_license":
			evidenceType = "license_text"
		case "public_domain":
			evidenceType = "source_page"
		}
	}
	if !oneOf(evidenceType, "license_text", "source_page", "purchase_receipt", "permission_email", "contract", "author_declaration", "screenshot", "hash_record", "repository_metadata", "other") {
		writeError(w, http.StatusBadRequest, "证据类型不正确。", nil)
		return
	}
	if b.EvidenceText == "" {
		b.EvidenceText = "发布者权利声明：本人声明该内容的权利类型为 " + b.OwnershipBasis + "，拟使用范围为 " + b.UseScope + "。该声明由发布者提交，尚未经过独立核验。"
	}
	storageMeta := map[string]any{"submittedBy": u.ID, "submittedByDisplayName": u.DisplayName, "submissionChannel": "publisher_portal", "rightsClaim": claim, "externalEvidenceUri": b.EvidenceURI}
	var uploadedEvidenceHash string
	if len(evidenceBytes) > 0 {
		if err = os.MkdirAll(a.resourceDir, 0700); err != nil {
			a.dbError(w, err, "无法准备证据文件存储目录。")
			return
		}
		ext := strings.ToLower(filepath.Ext(b.EvidenceFilename))
		storedName := "rights-evidence-" + randomUUID() + ext
		storedEvidencePath = filepath.Join(a.resourceDir, storedName)
		file, openErr := os.OpenFile(storedEvidencePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if openErr != nil {
			a.dbError(w, openErr, "无法保存证据文件。")
			return
		}
		if _, err = file.Write(evidenceBytes); err != nil {
			_ = file.Close()
			a.dbError(w, err, "保存证据文件失败。")
			return
		}
		if err = file.Close(); err != nil {
			a.dbError(w, err, "关闭证据文件失败。")
			return
		}
		sum := sha256.Sum256(evidenceBytes)
		uploadedEvidenceHash = hex.EncodeToString(sum[:])
		storageMeta["storagePath"] = storedName
		storageMeta["originalFilename"] = b.EvidenceFilename
		storageMeta["fileSizeBytes"] = len(evidenceBytes)
	}
	evidenceMeta, _ := json.Marshal(storageMeta)
	evidenceID := randomUUID()
	evidenceURI := b.EvidenceURI
	if storedEvidencePath != "" {
		evidenceURI = "/api/compliance/assets/" + assetID + "/evidence/" + evidenceID + "/download"
	}
	_, err = tx.ExecContext(r.Context(), "INSERT INTO portal.compliance_asset_evidence(evidence_id,asset_id,evidence_type,evidence_uri,evidence_text,sha256,verified,verified_by,metadata,notes) VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,FALSE,'',$7::jsonb,'Publisher-submitted evidence; requires independent verification.')", evidenceID, assetID, evidenceType, evidenceURI, b.EvidenceText, uploadedEvidenceHash, string(evidenceMeta))
	if err != nil {
		a.dbError(w, err, "保存证据材料失败。")
		return
	}
	_, err = tx.ExecContext(r.Context(), "INSERT INTO portal.compliance_audit_log(actor,action,object_type,object_id,before_state,after_state) VALUES($1,'publisher.rights_submitted',$2,$3::uuid,'{}'::jsonb,$4::jsonb)", u.ID, kind, assetID, string(claimJSON))
	if err != nil {
		a.dbError(w, err, "记录版权材料提交日志失败。")
		return
	}
	if err = tx.Commit(); err != nil {
		a.dbError(w, err, "提交版权材料失败。")
		return
	}
	transactionCommitted = true
	writeJSON(w, http.StatusCreated, map[string]any{"assetId": assetID, "reviewStatus": "pending", "message": "材料已提交，仍需版权审核人员核验；提交声明不会自动批准发布。"})
}
