package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type rightsGateError struct{ message string }

func (e rightsGateError) Error() string { return e.message }

func copyrightReviewAllowsPublish(status string) bool { return status == "approved" }

func contentBodyHash(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

type complianceSQLExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (a *portalAPI) syncContentComplianceAsset(ctx context.Context, db complianceSQLExecutor, item map[string]any) error {
	id := strings.TrimSpace(fmt.Sprint(item["id"]))
	if id == "" || id == "<nil>" {
		return fmt.Errorf("missing content id")
	}
	title := strings.TrimSpace(fmt.Sprint(item["title"]))
	body := fmt.Sprint(item["body"])
	if body == "<nil>" {
		body = ""
	}
	slug := fmt.Sprint(item["slug"])
	contentType := fmt.Sprint(item["contentType"])
	status := fmt.Sprint(item["status"])
	visibility := fmt.Sprint(item["visibility"])
	sum := sha256.Sum256([]byte(body))
	hash := hex.EncodeToString(sum[:])
	active := status == "published" && visibility == "public"
	distribution := "not_distributed"
	if active {
		distribution = "served_publicly"
	}
	metadata, _ := json.Marshal(map[string]any{
		"record_id": id, "title": title, "slug": slug, "content_type": contentType,
		"summary": fmt.Sprint(item["summary"]), "status": status, "visibility": visibility, "body_chars": len([]rune(body)),
		"source": "content_api_publish_gate", "rights_review_required": true,
	})
	assetPath := "db://portal.portal_content/" + id
	_, err := db.ExecContext(ctx, "UPDATE portal.compliance_assets SET published_or_active=FALSE,distribution_status='not_distributed',metadata=metadata || jsonb_build_object('superseded',true),notes=CASE WHEN notes='' THEN 'Superseded by a new content revision; retained for audit history.' ELSE notes || ' | Superseded by a new content revision; retained for audit history.' END,updated_at=now() WHERE asset_path=$1 AND sha256<>$2 AND discovered_by='database_scan'", assetPath, hash)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "INSERT INTO portal.compliance_assets (asset_type,asset_name,asset_path,ownership_basis,permission_status,use_scope,distribution_status,review_status,risk_level,sha256,discovered_by,published_or_active,metadata,notes) VALUES ('article',$1,$2,'unknown','unknown','commercial_web',$3,'pending','unknown',$4,'database_scan',$5,$6::jsonb,'New or changed content requires rights evidence and approval before publication.') ON CONFLICT (asset_path,sha256) WHERE asset_path <> '' DO UPDATE SET asset_name=EXCLUDED.asset_name,review_status=CASE WHEN compliance_assets.metadata ? 'title' AND (compliance_assets.metadata->>'title' IS DISTINCT FROM EXCLUDED.metadata->>'title' OR compliance_assets.metadata->>'slug' IS DISTINCT FROM EXCLUDED.metadata->>'slug' OR compliance_assets.metadata->>'content_type' IS DISTINCT FROM EXCLUDED.metadata->>'content_type' OR compliance_assets.metadata->>'summary' IS DISTINCT FROM EXCLUDED.metadata->>'summary') THEN 'pending' ELSE compliance_assets.review_status END,distribution_status=EXCLUDED.distribution_status,published_or_active=EXCLUDED.published_or_active,metadata=EXCLUDED.metadata,last_scanned_at=now(),updated_at=now()", title, assetPath, distribution, hash, active, string(metadata))
	return err
}

func (a *portalAPI) resourceAssetHash(storedFilename, title, description, resourceURL string) (string, error) {
	if storedFilename != "" {
		p := filepath.Join(a.resourceDir, filepath.Base(storedFilename))
		f, err := os.Open(p)
		if err != nil {
			return "", fmt.Errorf("open stored resource for hash: %w", err)
		}
		h := sha256.New()
		_, copyErr := io.Copy(h, f)
		closeErr := f.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	sum := sha256.Sum256([]byte(resourceURL + "\n" + title + "\n" + description))
	return hex.EncodeToString(sum[:]), nil
}

func (a *portalAPI) syncResourceComplianceAsset(ctx context.Context, db complianceSQLExecutor, id, title, description, resourceType, filename, mimeType, storedFilename, resourceURL string, isPublic bool, fileSize int64) error {
	assetPath := "db://portal.teaching_resources/" + id
	hash, hashErr := a.resourceAssetHash(storedFilename, title, description, resourceURL)
	if hashErr != nil {
		return hashErr
	}
	distribution := "not_distributed"
	if isPublic {
		distribution = "served_publicly"
	}
	metadata, _ := json.Marshal(map[string]any{
		"record_id": id, "title": title, "description": description,
		"resource_type": resourceType, "filename": filename, "mime_type": mimeType,
		"stored_filename": storedFilename, "resource_url": resourceURL,
		"file_size_bytes": fileSize, "source": "resource_api_publish_gate", "rights_review_required": true,
	})
	risk := "medium"
	lowerName := strings.ToLower(filename)
	if strings.HasSuffix(lowerName, ".mp3") || strings.HasSuffix(lowerName, ".mp4") || strings.HasSuffix(lowerName, ".pdf") || strings.HasSuffix(lowerName, ".docx") {
		risk = "high"
	}
	_, err := db.ExecContext(ctx, "UPDATE portal.compliance_assets SET published_or_active=FALSE,distribution_status='not_distributed',metadata=metadata || jsonb_build_object('superseded',true),notes=CASE WHEN notes='' THEN 'Superseded by a new resource revision; retained for audit history.' ELSE notes || ' | Superseded by a new resource revision; retained for audit history.' END,updated_at=now() WHERE asset_path=$1 AND sha256<>$2 AND discovered_by='database_scan'", assetPath, hash)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "INSERT INTO portal.compliance_assets (asset_type,asset_name,asset_path,source_url,ownership_basis,permission_status,use_scope,distribution_status,review_status,risk_level,sha256,file_size_bytes,discovered_by,published_or_active,metadata,notes) VALUES ('education_resource',$1,$2,$3,'unknown','unknown','commercial_web',$4,'pending',$5,$6,$7,'database_scan',$8,$9::jsonb,'New or changed resource requires rights evidence and approval before public distribution.') ON CONFLICT (asset_path,sha256) WHERE asset_path <> '' DO UPDATE SET asset_name=EXCLUDED.asset_name,source_url=EXCLUDED.source_url,review_status=CASE WHEN compliance_assets.metadata ? 'title' AND (compliance_assets.metadata->>'title' IS DISTINCT FROM EXCLUDED.metadata->>'title' OR compliance_assets.metadata->>'description' IS DISTINCT FROM EXCLUDED.metadata->>'description' OR compliance_assets.metadata->>'resource_type' IS DISTINCT FROM EXCLUDED.metadata->>'resource_type' OR compliance_assets.metadata->>'filename' IS DISTINCT FROM EXCLUDED.metadata->>'filename' OR compliance_assets.metadata->>'mime_type' IS DISTINCT FROM EXCLUDED.metadata->>'mime_type' OR compliance_assets.metadata->>'resource_url' IS DISTINCT FROM EXCLUDED.metadata->>'resource_url') THEN 'pending' ELSE compliance_assets.review_status END,risk_level=EXCLUDED.risk_level,distribution_status=EXCLUDED.distribution_status,file_size_bytes=EXCLUDED.file_size_bytes,published_or_active=EXCLUDED.published_or_active,metadata=EXCLUDED.metadata,last_scanned_at=now(),updated_at=now()", title, assetPath, resourceURL, distribution, risk, hash, fileSize, isPublic, string(metadata))
	return err
}

// canPublishComplianceAsset checks the current revision only. It never changes
// or deletes public content; it refuses a new publication until human review.
func (a *portalAPI) canPublishComplianceAsset(ctx context.Context, db complianceSQLExecutor, assetPath, expectedHash string) (string, error) {
	var id, ownership, permission, review, useScope string
	var licenseID sql.NullString
	var permissionExpiry sql.NullString
	err := db.QueryRowContext(ctx, "SELECT asset_id::text,ownership_basis,permission_status,review_status,license_id::text,use_scope,metadata->'publisher_rights_submission'->>'permissionExpiry' FROM portal.compliance_assets WHERE asset_path=$1 AND sha256=$2 LIMIT 1", assetPath, expectedHash).Scan(&id, &ownership, &permission, &review, &licenseID, &useScope, &permissionExpiry)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", rightsGateError{message: "当前内容版本尚未建立版权资产记录，请先进入版权合规中心完成盘点"}
		}
		return "", err
	}
	if !copyrightReviewAllowsPublish(review) {
		return "", rightsGateError{message: fmt.Sprintf("版权审核尚未通过（当前状态：%s）", review)}
	}
	if useScope == "internal_only" {
		return "", rightsGateError{message: "当前权利声明仅允许内部使用，不允许发布到公开网站。"}
	}
	if permissionExpiry.Valid && strings.TrimSpace(permissionExpiry.String) != "" {
		expiry, parseErr := time.Parse("2006-01-02", permissionExpiry.String)
		if parseErr != nil || time.Now().UTC().After(expiry.Add(24*time.Hour-time.Nanosecond)) {
			return "", rightsGateError{message: "授权有效期已过或无法核验，请补充有效授权后重新审核。"}
		}
	}
	if permission != "confirmed" && permission != "conditional" {
		return "", rightsGateError{message: fmt.Sprintf("授权状态尚未核实（当前状态：%s）", permission)}
	}
	if ownership == "unknown" || ownership == "" {
		return "", rightsGateError{message: "尚未确认原创、授权或其他权利依据"}
	}
	var evidenceOK bool
	switch ownership {
	case "original", "commissioned":
		err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM portal.compliance_asset_evidence WHERE asset_id=$1::uuid AND verified=TRUE AND evidence_type IN ('author_declaration','contract','legal_review'))", id).Scan(&evidenceOK)
	case "licensed", "third_party_permission":
		err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM portal.compliance_asset_evidence WHERE asset_id=$1::uuid AND verified=TRUE AND evidence_type IN ('license_text','permission_email','contract','purchase_receipt','legal_review'))", id).Scan(&evidenceOK)
	case "open_license":
		if !licenseID.Valid {
			return "", rightsGateError{message: "开放许可证资产必须关联许可证目录记录"}
		}
		var licenseReviewed, commercialAllowed bool
		err = db.QueryRowContext(ctx, "SELECT review_status='reviewed',commercial_use_allowed=TRUE FROM portal.compliance_license_catalog WHERE license_id=$1::uuid", licenseID.String).Scan(&licenseReviewed, &commercialAllowed)
		if err != nil {
			return "", err
		}
		if !licenseReviewed || !commercialAllowed {
			return "", rightsGateError{message: "关联许可证尚未完成商业用途审核"}
		}
		err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM portal.compliance_asset_evidence WHERE asset_id=$1::uuid AND verified=TRUE AND evidence_type IN ('license_text','repository_metadata','legal_review'))", id).Scan(&evidenceOK)
	case "public_domain":
		err = db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM portal.compliance_asset_evidence WHERE asset_id=$1::uuid AND verified=TRUE AND evidence_type IN ('source_page','legal_review'))", id).Scan(&evidenceOK)
	default:
		return "", rightsGateError{message: "权利依据类型尚未通过发布门槛核验"}
	}
	if err != nil {
		return "", err
	}
	if !evidenceOK {
		return "", rightsGateError{message: "缺少已核验的授权/来源证据；仅填写来源链接不足以通过发布审核"}
	}
	if strings.HasPrefix(assetPath, "db://portal.portal_content/") {
		contentID := strings.TrimPrefix(assetPath, "db://portal.portal_content/")
		var body, summary string
		if err := db.QueryRowContext(ctx, "SELECT body,summary FROM portal.portal_content WHERE content_id=$1::uuid", contentID).Scan(&body, &summary); err != nil {
			return "", err
		}
		seen := map[string]bool{}
		for _, match := range uploadedImageURLPattern.FindAllStringSubmatch(body, -1) {
			if len(match) < 2 || seen[match[1]] {
				continue
			}
			filename := match[1]
			seen[filename] = true
			var imageHash string
			if err := db.QueryRowContext(ctx, "SELECT sha256 FROM portal.uploaded_image_assets WHERE filename=$1", filename).Scan(&imageHash); err != nil {
				if err == sql.ErrNoRows {
					return "", rightsGateError{message: "文章包含未登记的站内图片：" + filename}
				}
				return "", err
			}
			if _, err := a.canPublishComplianceAsset(ctx, db, "/uploads/"+filename, imageHash); err != nil {
				return "", rightsGateError{message: "文章图片尚未通过独立版权审核（" + filename + "）： " + err.Error()}
			}
		}
		externalSeen := map[string]bool{}
		for _, rawURL := range externalContentURLPattern.FindAllString(body+" "+summary, -1) {
			imageURL := strings.TrimRight(rawURL, ".,")
			if !strings.Contains(imageURL, "/_next/image") && !strings.Contains(imageURL, "cdn.sanity.io/images/") && !externalImageFilePattern.MatchString(imageURL) {
				continue
			}
			if externalSeen[imageURL] {
				continue
			}
			externalSeen[imageURL] = true
			var externalAssetPath, externalHash string
			if err := db.QueryRowContext(ctx, "SELECT asset_path,sha256 FROM portal.compliance_assets WHERE asset_type='external_link' AND source_url=$1 ORDER BY updated_at DESC LIMIT 1", imageURL).Scan(&externalAssetPath, &externalHash); err != nil {
				if err == sql.ErrNoRows {
					return "", rightsGateError{message: "文章包含未登记的外部图片，需先登记来源和权利依据：" + imageURL}
				}
				return "", err
			}
			if _, err := a.canPublishComplianceAsset(ctx, db, externalAssetPath, externalHash); err != nil {
				return "", rightsGateError{message: "文章外部图片尚未通过独立版权审核：" + imageURL + "；" + err.Error()}
			}
		}
	}
	return id, nil
}
