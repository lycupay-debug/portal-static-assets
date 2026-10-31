package main

// Content collection handlers own author-scoped list, trash-list, and create operations. Creation keeps body-image reference locks and its database transaction together.

import (
	"errors"
	"fmt"
	"net/http"
)

func (a *portalAPI) listContent(w http.ResponseWriter, r *http.Request, u *portalUser) {
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
}

func (a *portalAPI) listContentTrash(w http.ResponseWriter, r *http.Request, u *portalUser) {
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
}

func (a *portalAPI) createContent(w http.ResponseWriter, r *http.Request, u *portalUser) {
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
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dbError(w, err, "内容保存失败。")
		return
	}
	defer tx.Rollback()
	body, _ := values["body"].(string)
	if err := lockContentImageReferences(r.Context(), tx, "", body); err != nil {
		if errors.Is(err, errManagedImageNotRegistered) {
			writeError(w, 400, "正文包含不可用的图片引用。", nil)
		} else {
			a.dbError(w, err, "校验正文图片引用失败。")
		}
		return
	}
	var newID string
	if err := tx.QueryRowContext(r.Context(), `INSERT INTO portal.portal_content(author_id,title,slug,content_type,tags,parent_category,child_category,summary,body,visibility) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING content_id::text`, u.ID, values["title"], values["slug"], values["contentType"], values["tags"], values["parentCategory"], values["childCategory"], values["summary"], values["body"], values["visibility"]).Scan(&newID); err != nil {
		a.dbError(w, err, "内容保存失败。")
		return
	}
	if err := tx.Commit(); err != nil {
		a.dbError(w, err, "内容保存失败。")
		return
	}
	item, err := oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT `+contentSelect+` FROM portal.portal_content c WHERE c.content_id=$1) x`, newID)
	if err != nil {
		a.dbError(w, err, "内容已保存，但读取结果失败。")
		return
	}
	writeJSON(w, 201, map[string]any{"item": item})
}
