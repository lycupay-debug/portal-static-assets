package main

// Content item routes own ID validation, owner-scoped reads, trash/restore, permanent deletion, version listing/restoration dispatch, and publish-state routing. Image cleanup invariants remain in the same transactional handlers.

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

func (a *portalAPI) contentByID(w http.ResponseWriter, r *http.Request, u *portalUser) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "content" || !portalUUID.MatchString(parts[1]) {
		writeError(w, 404, "未找到内容接口。", nil)
		return
	}
	id := parts[1]
	action := ""
	if len(parts) > 2 {
		action = strings.Join(parts[2:], "/")
	}
	ctx := r.Context()
	if action == "versions" && r.Method == http.MethodGet {
		items, err := queryJSONRows(ctx, a.db, `SELECT to_jsonb(x) FROM (SELECT version_id::text AS "versionId",version_number AS "versionNumber",snapshot->>'title' AS title,snapshot->>'slug' AS slug,snapshot->>'status' AS status,created_at AS "createdAt" FROM portal.portal_content_versions WHERE content_id=$1 AND author_id=$2 ORDER BY version_number DESC LIMIT 100) x`, id, u.ID)
		if err != nil {
			a.dbError(w, err, "无法读取历史版本。")
			return
		}
		var exists bool
		if err = a.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM portal.portal_content WHERE content_id=$1 AND author_id=$2)`, id, u.ID).Scan(&exists); err != nil || !exists {
			writeError(w, 404, "内容不存在或无权访问。", nil)
			return
		}
		writeJSON(w, 200, map[string]any{"items": items})
		return
	}
	if action == "restore" && r.Method == http.MethodPost {
		res, err := a.db.ExecContext(ctx, `UPDATE portal.portal_content SET deleted_at=NULL,updated_at=now() WHERE content_id=$1 AND author_id=$2 AND deleted_at IS NOT NULL`, id, u.ID)
		if err != nil {
			a.dbError(w, err, "恢复失败。")
			return
		}
		n, err := res.RowsAffected()
		if err != nil {
			a.dbError(w, err, "恢复失败。")
			return
		}
		if n == 0 {
			writeError(w, 404, "回收站中不存在该内容。", nil)
			return
		}
		item, err := oneJSON(ctx, a.db, `SELECT to_jsonb(x) FROM (SELECT `+contentSelect+` FROM portal.portal_content c WHERE c.content_id=$1) x`, id)
		if err != nil {
			a.dbError(w, err, "恢复失败。")
			return
		}
		writeJSON(w, 200, map[string]any{"item": item})
		return
	}
	if action == "permanent" && r.Method == http.MethodDelete {
		tx, err := a.db.BeginTx(ctx, nil)
		if err != nil {
			a.dbError(w, err, "永久删除失败。")
			return
		}
		defer tx.Rollback()
		var deletedBody string
		if err = tx.QueryRowContext(ctx, `SELECT body FROM portal.portal_content WHERE content_id=$1 AND author_id=$2 AND deleted_at IS NOT NULL FOR UPDATE`, id, u.ID).Scan(&deletedBody); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, 404, "回收站中不存在该内容。", nil)
			} else {
				a.dbError(w, err, "读取待永久删除内容失败。")
			}
			return
		}
		// Collect only app-managed image URLs in this content. Legacy files that
		// are not referenced here are deliberately left untouched.
		imageNames := managedImageNames(deletedBody)
		if err := lockManagedImageNames(ctx, tx, imageNames); err != nil {
			a.dbError(w, err, "锁定图片引用关系失败，永久删除事务已取消。")
			return
		}
		// Remove the content and image registrations atomically. An image is
		// eligible for cleanup only when no remaining content (including trash)
		// references it, so shared images are never removed prematurely.
		if _, err = tx.ExecContext(ctx, `DELETE FROM portal.portal_content WHERE content_id=$1 AND author_id=$2 AND deleted_at IS NOT NULL`, id, u.ID); err != nil {
			a.dbError(w, err, "永久删除失败。")
			return
		}
		cleanupEventIDs := make([]string, 0, len(imageNames))
		for _, name := range imageNames {
			// Restorable snapshots are live references just like the current body.
			var referenced bool
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM portal.portal_content WHERE position($1 in body)>0) OR EXISTS(SELECT 1 FROM portal.portal_content_versions WHERE position($1 in COALESCE(snapshot->>'body',''))>0)`, "/uploads/"+name).Scan(&referenced); err != nil {
				a.dbError(w, err, "核验图片引用关系失败，永久删除事务已取消。")
				return
			}
			if referenced {
				continue
			}
			if _, err = tx.ExecContext(ctx, `DELETE FROM portal.uploaded_images WHERE filename=$1`, name); err != nil {
				a.dbError(w, err, "清理图片登记失败，永久删除事务已取消。")
				return
			}
			eventID, enqueueErr := enqueueFileCleanup(ctx, tx, "upload_image", name)
			if enqueueErr != nil {
				a.dbError(w, enqueueErr, "登记图片清理任务失败，永久删除事务已取消。")
				return
			}
			cleanupEventIDs = append(cleanupEventIDs, eventID)
		}
		if err = tx.Commit(); err != nil {
			a.dbError(w, err, "永久删除事务提交失败。")
			return
		}
		// Durable events are committed with the content deletion. The C core
		// notification is only a wakeup; PostgreSQL recovery scans cover misses.
		for _, eventID := range cleanupEventIDs {
			go a.notifyEvent(r.Context(), eventID, fileCleanupEventType)
		}
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	if strings.HasPrefix(action, "versions/") && strings.HasSuffix(action, "/restore") && r.Method == http.MethodPost {
		versionID := strings.TrimSuffix(strings.TrimPrefix(action, "versions/"), "/restore")
		if !portalUUID.MatchString(versionID) {
			writeError(w, 404, "历史版本不存在。", nil)
			return
		}
		a.restoreVersion(w, r, u, id, versionID)
		return
	}
	if (action == "publish" || action == "unpublish") && r.Method == http.MethodPost {
		if action == "unpublish" {
			tx, err := a.db.BeginTx(ctx, nil)
			if err != nil {
				a.dbError(w, err, "取消发布失败。")
				return
			}
			defer tx.Rollback()
			res, err := tx.ExecContext(ctx, `UPDATE portal.portal_content SET status='unpublished',updated_at=now() WHERE content_id=$1 AND author_id=$2 AND deleted_at IS NULL`, id, u.ID)
			if err != nil {
				a.dbError(w, err, "取消发布失败。")
				return
			}
			n, err := res.RowsAffected()
			if err != nil {
				a.dbError(w, err, "取消发布失败。")
				return
			}
			if n == 0 {
				writeError(w, 404, "内容不存在或无权访问。", nil)
				return
			}
			item, err := oneJSON(ctx, tx, `SELECT to_jsonb(x) FROM (SELECT `+contentSelect+` FROM portal.portal_content c WHERE c.content_id=$1 AND c.author_id=$2) x`, id, u.ID)
			if err != nil {
				a.dbError(w, err, "读取取消发布结果失败，事务已取消。")
				return
			}
			if err = tx.Commit(); err != nil {
				a.dbError(w, err, "取消发布失败。")
				return
			}
			writeJSON(w, 200, map[string]any{"item": item})
			return
		}
		a.publishContent(w, r, u, id)
		return
	}
	if action != "" {
		writeError(w, 404, "未找到内容接口。", nil)
		return
	}
	if r.Method == http.MethodGet {
		item, err := oneJSON(ctx, a.db, `SELECT to_jsonb(x) FROM (SELECT `+contentSelect+` FROM portal.portal_content c WHERE c.content_id=$1 AND c.author_id=$2 AND c.deleted_at IS NULL) x`, id, u.ID)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, 404, "内容不存在或无权访问。", nil)
			return
		}
		if err != nil {
			a.dbError(w, err, "读取内容失败。")
			return
		}
		writeJSON(w, 200, map[string]any{"item": item})
		return
	}
	if r.Method == http.MethodDelete {
		res, err := a.db.ExecContext(ctx, `UPDATE portal.portal_content SET deleted_at=now(),updated_at=now() WHERE content_id=$1 AND author_id=$2 AND deleted_at IS NULL`, id, u.ID)
		if err != nil {
			a.dbError(w, err, "删除失败。")
			return
		}
		n, err := res.RowsAffected()
		if err != nil {
			a.dbError(w, err, "删除失败。")
			return
		}
		if n == 0 {
			writeError(w, 404, "内容不存在或无权访问。", nil)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	if r.Method == http.MethodPatch {
		a.patchContent(w, r, u, id)
		return
	}
	writeError(w, 405, "不支持的请求方式。", nil)
}
