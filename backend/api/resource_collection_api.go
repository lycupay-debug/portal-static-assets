package main

// Resource collection handlers own active/trash listing and resource creation.
// They use shared database, response, validation, and managed-file helpers; create
// preserves the existing file/transaction cleanup rules and ambiguous-COMMIT policy.

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

func (a *portalAPI) listResources(w http.ResponseWriter, r *http.Request, u *portalUser) {
	q := `SELECT to_jsonb(x) FROM (SELECT resource_id::text AS "id",title,description,resource_type AS "type",grade AS "grade",is_public AS "isPublic",resource_url AS "url",original_filename AS "filename",mime_type AS "mimeType",file_size AS "fileSize",created_at AS "createdAt",updated_at AS "updatedAt" FROM portal.teaching_resources WHERE owner_id=$1 AND deleted_at IS NULL`
	args := []any{u.ID}
	orderBy := "updated_at DESC"
	if search := normalizeSearchQuery(r.URL.Query().Get("q")); search != "" {
		args = append(args, search)
		placeholder := fmt.Sprintf("$%d", len(args))
		orderBy = resourceSearchOrderSQL(placeholder) + ", " + resourceSearchSimilarityOrderSQL("", placeholder) + ", updated_at DESC"
		q += ` AND (` + resourceDirectSearchMatchSQL("", placeholder) + ` OR (NOT EXISTS (SELECT 1 FROM portal.teaching_resources matched WHERE matched.owner_id=$1 AND matched.deleted_at IS NULL AND ` + resourceDirectSearchMatchSQL("matched", placeholder) + `) AND ` + resourceFuzzySearchMatchSQL("", placeholder) + `))`
	}
	q += ` ORDER BY ` + orderBy + ` LIMIT 500) x`
	items, err := queryJSONRows(r.Context(), a.db, q, args...)
	if err != nil {
		a.dbError(w, err, "读取资源失败。")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
	return
}

func (a *portalAPI) listResourceTrash(w http.ResponseWriter, r *http.Request, u *portalUser) {
	q := `SELECT to_jsonb(x) FROM (SELECT resource_id::text AS "id",title,description,resource_type AS "type",grade AS "grade",is_public AS "isPublic",resource_url AS "url",original_filename AS "filename",mime_type AS "mimeType",file_size AS "fileSize",created_at AS "createdAt",deleted_at AS "deletedAt" FROM portal.teaching_resources WHERE owner_id=$1 AND deleted_at IS NOT NULL`
	args := []any{u.ID}
	orderBy := "deleted_at DESC"
	if search := normalizeSearchQuery(r.URL.Query().Get("q")); search != "" {
		args = append(args, search)
		placeholder := fmt.Sprintf("$%d", len(args))
		orderBy = resourceSearchOrderSQL(placeholder) + ", " + resourceSearchSimilarityOrderSQL("", placeholder) + ", deleted_at DESC"
		q += ` AND (` + resourceDirectSearchMatchSQL("", placeholder) + ` OR (NOT EXISTS (SELECT 1 FROM portal.teaching_resources matched WHERE matched.owner_id=$1 AND matched.deleted_at IS NOT NULL AND ` + resourceDirectSearchMatchSQL("matched", placeholder) + `) AND ` + resourceFuzzySearchMatchSQL("", placeholder) + `))`
	}
	q += ` ORDER BY ` + orderBy + ` LIMIT 500) x`
	items, err := queryJSONRows(r.Context(), a.db, q, args...)
	if err != nil {
		a.dbError(w, err, "读取回收站失败。")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
	return
}

func (a *portalAPI) createResource(w http.ResponseWriter, r *http.Request, u *portalUser) {
	var b resourceInput
	if err := decodeJSON(w, r, maxResourceBody, &b); err != nil {
		writeError(w, 400, "请求格式不正确。", nil)
		return
	}
	data, filename, mimeType, err := validateResourceFile(b)
	if err != nil {
		writeError(w, 400, err.Error(), nil)
		return
	}
	if len(data) > maxResourceFile {
		writeError(w, 413, "单个文件大小不能超过 25 MB。", nil)
		return
	}
	stored, err := a.storeResourceFile(data, filename)
	if err != nil {
		a.dbError(w, err, "资源文件保存失败。")
		return
	}
	var fileSize any
	if len(data) > 0 {
		fileSize = len(data)
	}
	var mimeAny any
	if mimeType != "" {
		mimeAny = mimeType
	}
	var filenameAny any
	if filename != "" {
		filenameAny = filename
	}
	var storedAny any
	if stored != "" {
		storedAny = stored
	}
	var urlAny any
	if strings.TrimSpace(b.URL) != "" {
		urlAny = strings.TrimSpace(b.URL)
	}
	var resourceID string
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		if stored != "" {
			_ = removeManagedFile(a.resourceDir, stored)
		}
		a.dbError(w, err, "开始资源保存事务失败。")
		return
	}
	err = tx.QueryRowContext(r.Context(), `INSERT INTO portal.teaching_resources(owner_id,title,description,resource_type,grade,is_public,resource_url,original_filename,stored_filename,mime_type,file_size) VALUES($1,$2,$3,$4,$5,COALESCE($6,FALSE),$7,$8,$9,$10,$11) RETURNING resource_id::text`, u.ID, strings.TrimSpace(b.Title), strings.TrimSpace(b.Description), resourceType(b.Type), nullableString(resourceGrade(b.Grade)), b.IsPublic, urlAny, filenameAny, storedAny, mimeAny, fileSize).Scan(&resourceID)
	if err != nil {
		_ = tx.Rollback()
		if stored != "" {
			_ = removeManagedFile(a.resourceDir, stored)
		}
		a.dbError(w, err, "资源保存失败。")
		return
	}
	item, err := oneJSON(r.Context(), tx, `SELECT to_jsonb(x) FROM (SELECT resource_id::text AS "id",title,description,resource_type AS "type",grade AS "grade",is_public AS "isPublic",resource_url AS "url",original_filename AS "filename",mime_type AS "mimeType",file_size AS "fileSize",created_at AS "createdAt",updated_at AS "updatedAt" FROM portal.teaching_resources WHERE resource_id=$1 AND owner_id=$2) x`, resourceID, u.ID)
	if err != nil {
		_ = tx.Rollback()
		if stored != "" {
			_ = removeManagedFile(a.resourceDir, stored)
		}
		a.dbError(w, err, "读取资源保存结果失败，事务已取消。")
		return
	}
	if err = tx.Commit(); err != nil {
		// A failed COMMIT can have an ambiguous outcome. Keep the managed file
		// until the database row can be reconciled; never create a broken row.
		fmt.Fprintf(os.Stderr, "portal_api_error operation=%q resource_id=%q error=%v\n", "resource create commit", resourceID, err)
		writeError(w, http.StatusServiceUnavailable, "资源保存结果暂时无法确认，文件已保留，请刷新资源列表核验。", nil)
		return
	}
	writeJSON(w, 201, map[string]any{"item": item})
	return
}
