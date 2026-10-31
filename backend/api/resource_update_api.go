package main

// Resource PATCH handling owns update validation and file replacement. The row
// lock serializes concurrent replacements; cleanup events and ambiguous-COMMIT
// file preservation remain part of the transaction contract.

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
)

func (a *portalAPI) patchResource(w http.ResponseWriter, r *http.Request, u *portalUser, id string) {
	var b resourceInput
	if err := decodeJSON(w, r, maxResourceBody, &b); err != nil {
		writeError(w, 400, "请求格式不正确。", nil)
		return
	}
	b.Title = strings.TrimSpace(b.Title)
	b.Description = strings.TrimSpace(b.Description)
	b.Type = resourceType(b.Type)
	b.Grade = resourceGrade(b.Grade)
	if b.Title == "" || runeLen(b.Title) > 200 || runeLen(b.Description) > 2000 || runeLen(b.Type) > 50 || b.Grade == "__invalid_grade__" {
		writeError(w, 400, "资源名称、类型、年级或资源说明不正确。", nil)
		return
	}
	if b.URL != "" && (!strings.HasPrefix(strings.ToLower(strings.TrimSpace(b.URL)), "http://") && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(b.URL)), "https://") || len(b.URL) > 2048) {
		writeError(w, 400, "资源 URL 必须是有效的 HTTP/HTTPS 链接，且不超过 2048 个字符。", nil)
		return
	}

	// Serialize updates before creating a replacement file. Without FOR UPDATE,
	// concurrent requests can all copy the same old filename, write independent
	// replacements, and leave every losing replacement orphaned on disk.
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dbError(w, err, "开始资源更新事务失败。")
		return
	}
	defer tx.Rollback()

	var old sql.NullString
	var currentFilename sql.NullString
	err = tx.QueryRowContext(r.Context(), `SELECT stored_filename,original_filename FROM portal.teaching_resources WHERE resource_id=$1 AND owner_id=$2 AND deleted_at IS NULL FOR UPDATE`, id, u.ID).Scan(&old, &currentFilename)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, 404, "资源不存在、已删除或无权编辑。", nil)
		} else {
			a.dbError(w, err, "读取资源当前状态失败。")
		}
		return
	}

	var data []byte
	var name, mt string
	if b.Data != "" || strings.TrimSpace(b.URL) != "" {
		data, name, mt, err = validateResourceFile(resourceInput{Title: b.Title, Description: b.Description, Type: b.Type, URL: b.URL, Filename: b.Filename, Data: b.Data})
		if err != nil {
			writeError(w, 400, err.Error(), nil)
			return
		}
	}
	if len(data) > maxResourceFile {
		writeError(w, 413, "单个文件大小不能超过 25 MB。", nil)
		return
	}
	if len(data) > 0 && old.Valid && old.String != "" && name == currentFilename.String {
		currentData, readErr := readManagedFile(a.resourceDir, old.String)
		if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
			a.dbError(w, readErr, "读取现有资源文件失败，未保存任何修改。")
			return
		}
		if readErr == nil {
			oldHash, newHash := sha256.Sum256(currentData), sha256.Sum256(data)
			if oldHash == newHash {
				data = nil
				name, mt = "", ""
			}
		}
		// A missing old file is repairable by uploading a replacement with the
		// same original filename; the cleanup event below is idempotent.
	}

	stored := old.String
	var newStored string
	keepNewStored := false
	defer func() {
		if newStored != "" && !keepNewStored {
			_ = removeManagedFile(a.resourceDir, newStored)
		}
	}()
	if len(data) > 0 {
		newStored, err = a.storeResourceFile(data, name)
		if err != nil {
			a.dbError(w, err, "资源文件保存失败。")
			return
		}
		stored = newStored
	}
	if stored == "" && strings.TrimSpace(b.URL) == "" {
		writeError(w, 400, "该资源没有上传文件，请保留 URL 链接或上传文件。", nil)
		return
	}
	var urlValue any
	if strings.TrimSpace(b.URL) != "" {
		urlValue = strings.TrimSpace(b.URL)
	}
	var fileNameValue, mimeValue, sizeValue any
	if len(data) > 0 {
		fileNameValue = name
		mimeValue = mt
		sizeValue = len(data)
	}
	res, err := tx.ExecContext(r.Context(), `UPDATE portal.teaching_resources SET title=$3,description=$4,resource_type=$5,grade=$6,is_public=COALESCE($7,is_public),resource_url=$8,original_filename=COALESCE($9,original_filename),stored_filename=$10,mime_type=COALESCE($11,mime_type),file_size=COALESCE($12,file_size),updated_at=now() WHERE resource_id=$1 AND owner_id=$2 AND deleted_at IS NULL`, id, u.ID, b.Title, b.Description, b.Type, nullableString(b.Grade), b.IsPublic, urlValue, fileNameValue, nullableString(stored), mimeValue, sizeValue)
	if err != nil {
		a.dbError(w, err, "资源更新失败。")
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		a.dbError(w, err, "资源更新失败。")
		return
	}
	if n == 0 {
		writeError(w, 404, "资源不存在或无权编辑。", nil)
		return
	}

	cleanupEventID := ""
	if newStored != "" && old.Valid && old.String != "" {
		cleanupEventID, err = enqueueFileCleanup(r.Context(), tx, "teaching_resource", old.String)
		if err != nil {
			a.dbError(w, err, "登记旧资源文件清理任务失败，更新事务已取消。")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		// A commit error can have an ambiguous outcome. Preserve the new file so
		// a possibly committed row never points to a file removed by this process.
		keepNewStored = true
		fmt.Fprintf(os.Stderr, "portal_api_error operation=%q resource_id=%q error=%v\n", "resource update commit", id, err)
		writeError(w, http.StatusServiceUnavailable, "资源更新结果暂时无法确认，新文件已保留，请检查资源记录和清理队列。", nil)
		return
	}
	keepNewStored = true
	if cleanupEventID != "" {
		go a.notifyEvent(r.Context(), cleanupEventID, fileCleanupEventType)
	}

	item, err := oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT resource_id::text AS "id",title,description,resource_type AS "type",grade AS "grade",is_public AS "isPublic",resource_url AS "url",original_filename AS "filename",mime_type AS "mimeType",file_size AS "fileSize",created_at AS "createdAt",updated_at AS "updatedAt" FROM portal.teaching_resources WHERE resource_id=$1) x`, id)
	if err != nil {
		a.dbError(w, err, "资源已更新，但读取结果失败。")
		return
	}
	writeJSON(w, 200, map[string]any{"item": item})
}
