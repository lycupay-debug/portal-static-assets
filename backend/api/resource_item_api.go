package main

// Resource item lifecycle handlers own download, restore, soft-delete, and
// permanent-delete behavior. Permanent deletion records managed-file cleanup in
// the database transaction and notifies the event core only after commit.

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
)

func (a *portalAPI) downloadResource(w http.ResponseWriter, r *http.Request, u *portalUser, id string) {
	var stored, filename, mtype sql.NullString
	err := a.db.QueryRowContext(r.Context(), `SELECT stored_filename,original_filename,mime_type FROM portal.teaching_resources WHERE resource_id=$1 AND owner_id=$2 AND deleted_at IS NULL`, id, u.ID).Scan(&stored, &filename, &mtype)
	if err != nil {
		writeError(w, 404, "资源不存在或无权访问。", nil)
		return
	}
	if !stored.Valid {
		writeError(w, 400, "该资源仅提供外部链接，请使用资源 URL。", nil)
		return
	}
	f, err := openManagedFile(a.resourceDir, stored.String)
	if err != nil {
		writeError(w, 404, "资源文件缺失。", nil)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", mtype.String)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(filename.String)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, f)
	return
}

func (a *portalAPI) restoreResource(w http.ResponseWriter, r *http.Request, u *portalUser, id string) {
	res, err := a.db.ExecContext(r.Context(), `UPDATE portal.teaching_resources SET deleted_at=NULL,updated_at=now() WHERE resource_id=$1 AND owner_id=$2 AND deleted_at IS NOT NULL`, id, u.ID)
	if err != nil {
		a.dbError(w, err, "资源恢复失败。")
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		a.dbError(w, err, "资源恢复失败。")
		return
	}
	if n == 0 {
		writeError(w, 404, "资源不存在或无权恢复。", nil)
		return
	}
	item, err := oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT resource_id::text AS "id",title,description,resource_type AS "type",grade AS "grade",is_public AS "isPublic",resource_url AS "url",original_filename AS "filename",mime_type AS "mimeType",file_size AS "fileSize",created_at AS "createdAt",updated_at AS "updatedAt" FROM portal.teaching_resources WHERE resource_id=$1) x`, id)
	if err != nil {
		a.dbError(w, err, "读取恢复结果失败。")
		return
	}
	writeJSON(w, 200, map[string]any{"item": item})
	return
}

func (a *portalAPI) permanentlyDeleteResource(w http.ResponseWriter, r *http.Request, u *portalUser, id string) {
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dbError(w, err, "资源永久删除失败。")
		return
	}
	defer tx.Rollback()
	var stored sql.NullString
	if err = tx.QueryRowContext(r.Context(), `SELECT stored_filename FROM portal.teaching_resources WHERE resource_id=$1 AND owner_id=$2 AND deleted_at IS NOT NULL FOR UPDATE`, id, u.ID).Scan(&stored); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, 404, "资源不存在或不在回收站。", nil)
		} else {
			a.dbError(w, err, "读取待永久删除资源失败。")
		}
		return
	}
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM portal.teaching_resources WHERE resource_id=$1 AND owner_id=$2 AND deleted_at IS NOT NULL`, id, u.ID); err != nil {
		a.dbError(w, err, "资源永久删除失败。")
		return
	}
	cleanupEventID := ""
	if stored.Valid {
		filename := filepath.Base(stored.String)
		if filename != stored.String {
			a.dbError(w, errors.New("stored resource filename is not a basename"), "资源文件路径异常，永久删除事务已取消。")
			return
		}
		cleanupEventID, err = enqueueFileCleanup(r.Context(), tx, "teaching_resource", filename)
		if err != nil {
			a.dbError(w, err, "登记资源文件清理任务失败，永久删除事务已取消。")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		a.dbError(w, err, "资源永久删除事务提交失败。")
		return
	}
	if cleanupEventID != "" {
		go a.notifyEvent(r.Context(), cleanupEventID, fileCleanupEventType)
	}
	writeJSON(w, 200, map[string]any{"ok": true})
	return
}

func (a *portalAPI) trashResource(w http.ResponseWriter, r *http.Request, u *portalUser, id string) {
	res, err := a.db.ExecContext(r.Context(), `UPDATE portal.teaching_resources SET deleted_at=now(),updated_at=now() WHERE resource_id=$1 AND owner_id=$2 AND deleted_at IS NULL`, id, u.ID)
	if err != nil {
		a.dbError(w, err, "删除资源失败。")
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		a.dbError(w, err, "删除资源失败。")
		return
	}
	if n == 0 {
		writeError(w, 404, "资源不存在或已在回收站。", nil)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
	return
}
