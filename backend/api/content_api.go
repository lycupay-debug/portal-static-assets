package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

func (a *portalAPI) content(w http.ResponseWriter, r *http.Request) {
	u, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	path := r.URL.Path
	switch {
	case path == "/content" && r.Method == http.MethodGet:
		status := r.URL.Query().Get("status")
		if status != "" && status != "draft" && status != "published" && status != "unpublished" {
			writeError(w, 400, "内容状态不正确。", nil)
			return
		}
		q := `SELECT to_jsonb(x) FROM (SELECT ` + contentSelect + `, (SELECT count(*)::int FROM portal.portal_content_interactions i WHERE i.content_id=c.content_id AND i.interaction_type='like') AS "likeCount", (SELECT count(*)::int FROM portal.portal_content_interactions i WHERE i.content_id=c.content_id AND i.interaction_type='favorite') AS "favoriteCount" FROM portal.portal_content c WHERE c.author_id=$1 AND c.deleted_at IS NULL`
		args := []any{u.ID}
		if status != "" {
			q += ` AND c.status=$2`
			args = append(args, status)
		}
		if search := normalizeSearchQuery(r.URL.Query().Get("q")); search != "" {
			args = append(args, search)
			placeholder := fmt.Sprintf("$%d", len(args))
			expr := `coalesce(c.title, '') || ' ' || coalesce(c.summary, '') || ' ' || coalesce(c.body, '') || ' ' || coalesce(c.slug, '') || ' ' || coalesce(portal.text_array_join(c.tags, ' '), '') || ' ' || coalesce(c.parent_category, '') || ' ' || coalesce(c.child_category, '')`
			fuzzyExpr := `coalesce(c.title, '') || ' ' || coalesce(c.summary, '') || ' ' || coalesce(c.slug, '') || ' ' || coalesce(portal.text_array_join(c.tags, ' '), '') || ' ' || coalesce(c.parent_category, '') || ' ' || coalesce(c.child_category, '')`
			q += ` AND (to_tsvector('simple', ` + expr + `) @@ websearch_to_tsquery('simple', ` + placeholder + `) OR lower(` + expr + `) LIKE '%' || lower(` + placeholder + `) || '%' OR regexp_replace(lower(` + expr + `), '[[:space:]]+', '', 'g') LIKE '%' || regexp_replace(lower(` + placeholder + `), '[[:space:]]+', '', 'g') || '%' OR lower(` + placeholder + `) <% lower(` + fuzzyExpr + `))`
		}
		q += ` ORDER BY c.updated_at DESC,c.content_id DESC LIMIT 500) x`
		items, err := queryJSONRows(r.Context(), a.db, q, args...)
		if err != nil {
			writeError(w, 500, "服务暂时不可用，请稍后再试。", nil)
			return
		}
		writeJSON(w, 200, map[string]any{"items": items})
	case path == "/content/trash" && r.Method == http.MethodGet:
		q := `SELECT to_jsonb(x) FROM (SELECT ` + contentSelect + `, (SELECT count(*)::int FROM portal.portal_content_interactions i WHERE i.content_id=c.content_id AND i.interaction_type='like') AS "likeCount", (SELECT count(*)::int FROM portal.portal_content_interactions i WHERE i.content_id=c.content_id AND i.interaction_type='favorite') AS "favoriteCount" FROM portal.portal_content c WHERE c.author_id=$1 AND c.deleted_at IS NOT NULL`
		args := []any{u.ID}
		if search := normalizeSearchQuery(r.URL.Query().Get("q")); search != "" {
			args = append(args, search)
			placeholder := fmt.Sprintf("$%d", len(args))
			expr := `coalesce(c.title, '') || ' ' || coalesce(c.summary, '') || ' ' || coalesce(c.body, '') || ' ' || coalesce(c.slug, '') || ' ' || coalesce(portal.text_array_join(c.tags, ' '), '') || ' ' || coalesce(c.parent_category, '') || ' ' || coalesce(c.child_category, '')`
			fuzzyExpr := `coalesce(c.title, '') || ' ' || coalesce(c.summary, '') || ' ' || coalesce(c.slug, '') || ' ' || coalesce(portal.text_array_join(c.tags, ' '), '') || ' ' || coalesce(c.parent_category, '') || ' ' || coalesce(c.child_category, '')`
			q += ` AND (to_tsvector('simple', ` + expr + `) @@ websearch_to_tsquery('simple', ` + placeholder + `) OR lower(` + expr + `) LIKE '%' || lower(` + placeholder + `) || '%' OR regexp_replace(lower(` + expr + `), '[[:space:]]+', '', 'g') LIKE '%' || regexp_replace(lower(` + placeholder + `), '[[:space:]]+', '', 'g') || '%' OR lower(` + placeholder + `) <% lower(` + fuzzyExpr + `))`
		}
		q += ` ORDER BY c.deleted_at DESC,c.content_id DESC LIMIT 500) x`
		items, err := queryJSONRows(r.Context(), a.db, q, args...)
		if err != nil {
			writeError(w, 500, "服务暂时不可用，请稍后再试。", nil)
			return
		}
		writeJSON(w, 200, map[string]any{"items": items})
	case path == "/content" && r.Method == http.MethodPost:
		var in portalContentInput
		if err := decodeJSON(w, r, maxContentBody, &in); err != nil {
			writeError(w, 400, "请求格式不正确。", nil)
			return
		}
		values, errs := validateContentInput(in, false)
		if len(errs) > 0 {
			writeError(w, 400, errs[0], errs)
			return
		}
		if len(values["tags"].([]string)) == 0 {
			writeError(w, 400, "请至少选择一个知识广场标签。", nil)
			return
		}
		var newID string
		err := a.db.QueryRowContext(r.Context(), `INSERT INTO portal.portal_content(author_id,title,slug,content_type,tags,parent_category,child_category,summary,body,visibility) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING content_id::text`, u.ID, values["title"], values["slug"], values["contentType"], values["tags"], values["parentCategory"], values["childCategory"], values["summary"], values["body"], values["visibility"]).Scan(&newID)
		if err != nil {
			a.dbError(w, err, "内容保存失败。")
			return
		}
		item, err := oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT `+contentSelect+` FROM portal.portal_content c WHERE c.content_id=$1) x`, newID)
		if err != nil {
			a.dbError(w, err, "内容已保存，但读取结果失败。")
			return
		}
		writeJSON(w, 201, map[string]any{"item": item})
	default:
		a.contentByID(w, r, u)
	}
}
func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
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
		n, _ := res.RowsAffected()
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
		imageMatches := uploadedImageURLPattern.FindAllStringSubmatch(deletedBody, -1)
		imageNames := make([]string, 0, len(imageMatches))
		seenImages := make(map[string]struct{}, len(imageMatches))
		for _, match := range imageMatches {
			if len(match) != 2 || !uploadedImageName.MatchString(match[1]) {
				continue
			}
			if _, exists := seenImages[match[1]]; exists {
				continue
			}
			seenImages[match[1]] = struct{}{}
			imageNames = append(imageNames, match[1])
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
			var referenced bool
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM portal.portal_content WHERE position($1 in body)>0)`, "/uploads/"+name).Scan(&referenced); err != nil {
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
			res, err := a.db.ExecContext(ctx, `UPDATE portal.portal_content SET status='unpublished',updated_at=now() WHERE content_id=$1 AND author_id=$2 AND deleted_at IS NULL`, id, u.ID)
			if err != nil {
				a.dbError(w, err, "取消发布失败。")
				return
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				writeError(w, 404, "内容不存在或无权访问。", nil)
				return
			}
			item, err := oneJSON(ctx, a.db, `SELECT to_jsonb(x) FROM (SELECT `+contentSelect+` FROM portal.portal_content c WHERE c.content_id=$1) x`, id)
			if err != nil {
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
		n, _ := res.RowsAffected()
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
func (a *portalAPI) patchContent(w http.ResponseWriter, r *http.Request, u *portalUser, id string) {
	var in portalContentInput
	if err := decodeJSON(w, r, maxContentBody, &in); err != nil {
		writeError(w, 400, "请求格式不正确。", nil)
		return
	}
	v, errs := validateContentInput(in, true)
	if len(errs) > 0 {
		writeError(w, 400, errs[0], errs)
		return
	}
	if len(v) == 0 {
		writeError(w, 400, "没有可更新的字段。", nil)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dbError(w, err, "保存失败。")
		return
	}
	defer tx.Rollback()
	cur, err := oneJSON(r.Context(), tx, `SELECT to_jsonb(x) FROM (SELECT `+contentSelect+` FROM portal.portal_content c WHERE c.content_id=$1 AND c.author_id=$2 AND c.deleted_at IS NULL FOR UPDATE) x`, id, u.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, 404, "内容不存在或无权访问。", nil)
		} else {
			a.dbError(w, err, "保存失败.")
		}
		return
	}
	mergedParent := cur["parentCategory"]
	mergedChild := cur["childCategory"]
	if val, ok := v["parentCategory"]; ok {
		mergedParent = val
	}
	if val, ok := v["childCategory"]; ok {
		mergedChild = val
	}
	if (mergedParent == nil) != (mergedChild == nil) {
		writeError(w, 400, "父类和子类必须同时填写或同时清空。", nil)
		return
	}
	if mergedParent != nil {
		p, pok := mergedParent.(string)
		c, cok := mergedChild.(string)
		if !pok || !cok || !contains(portalTaxonomy[p], c) {
			writeError(w, 400, "子类必须属于当前选择的父类。", nil)
			return
		}
	}
	changed := []string{}
	for k, nv := range v {
		if !jsonEqual(cur[k], nv) {
			changed = append(changed, k)
		}
	}
	if len(changed) == 0 {
		if err = tx.Commit(); err != nil {
			a.dbError(w, err, "保存失败。")
			return
		}
		writeJSON(w, 200, map[string]any{"item": cur, "versionCreated": false})
		return
	}

	if err = saveContentVersion(r.Context(), tx, cur, u.ID); err != nil {
		a.dbError(w, err, "保存历史版本失败。")
		return
	}
	sort.Strings(changed)
	sets := []string{}
	args := []any{id, u.ID}
	for _, k := range changed {
		col, ok := contentColumns[k]
		if !ok {
			continue
		}
		args = append(args, v[k])
		sets = append(sets, fmt.Sprintf("%s=$%d", col, len(args)))
	}
	q := `UPDATE portal.portal_content SET ` + strings.Join(sets, ",") + `,updated_at=now() WHERE content_id=$1 AND author_id=$2 RETURNING content_id::text`
	var savedID string
	// #nosec G701 -- column names are selected only from the compile-time contentColumns allowlist; all user values use SQL parameters.
	if err = tx.QueryRowContext(r.Context(), q, args...).Scan(&savedID); err != nil {
		a.dbError(w, err, "保存失败。")
		return
	}
	item, err := oneJSON(r.Context(), tx, `SELECT to_jsonb(x) FROM (SELECT `+contentSelect+` FROM portal.portal_content c WHERE c.content_id=$1) x`, savedID)
	if err != nil {
		a.dbError(w, err, "保存失败。")
		return
	}
	if err = tx.Commit(); err != nil {
		a.dbError(w, err, "保存失败。")
		return
	}
	writeJSON(w, 200, map[string]any{"item": item, "versionCreated": true})
}
func (a *portalAPI) publishContent(w http.ResponseWriter, r *http.Request, u *portalUser, id string) {
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dbError(w, err, "发布失败。")
		return
	}
	defer tx.Rollback()
	item, err := oneJSON(r.Context(), tx, `SELECT to_jsonb(x) FROM (SELECT `+contentSelect+` FROM portal.portal_content c WHERE c.content_id=$1 AND c.author_id=$2 AND c.deleted_at IS NULL FOR UPDATE) x`, id, u.ID)
	if err != nil {
		writeError(w, 404, "内容不存在或无权访问。", nil)
		return
	}
	if item["visibility"] != "public" {
		writeError(w, 400, "发布前请先将可见范围设置为公开。", nil)
		return
	}
	in := portalContentInput{}
	title := fmt.Sprint(item["title"])
	slug := fmt.Sprint(item["slug"])
	typ := fmt.Sprint(item["contentType"])
	body := fmt.Sprint(item["body"])
	summary := fmt.Sprint(item["summary"])
	vis := "public"
	in.Title = &title
	in.Slug = &slug
	in.ContentType = &typ
	in.Body = &body
	in.Summary = &summary
	in.Visibility = &vis
	tags := stringSlice(item["tags"])
	in.Tags = &tags
	if p, ok := item["parentCategory"].(string); ok {
		in.ParentCategory = &p
	}
	if c, ok := item["childCategory"].(string); ok {
		in.ChildCategory = &c
	}
	_, errs := validateContentInput(in, false)
	if len(errs) > 0 {
		writeError(w, 400, errs[0], errs)
		return
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE portal.portal_content SET status='published',published_at=now(),updated_at=now() WHERE content_id=$1 AND author_id=$2`, id, u.ID)
	if err != nil {
		a.dbError(w, err, "发布失败。")
		return
	}
	eid := randomUUID()
	payload, _ := json.Marshal(map[string]any{"title": title, "slug": slug, "content_type": typ, "tags": tags, "visibility": vis, "summary": summary, "body": body})
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal.event_inbox(event_id,event_type,payload) VALUES($1::uuid,'portal.content.validate',$2::jsonb)`, eid, string(payload)); err != nil {
		a.dbError(w, err, "发布事件登记失败。")
		return
	}
	if err = tx.Commit(); err != nil {
		a.dbError(w, err, "发布失败。")
		return
	}
	go a.notifyEvent(r.Context(), eid, "portal.content.validate")
	item, err = oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT `+contentSelect+` FROM portal.portal_content c WHERE c.content_id=$1) x`, id)
	if err != nil {
		a.dbError(w, err, "读取发布结果失败。")
		return
	}
	writeJSON(w, 200, map[string]any{"item": item, "eventId": eid})
}
