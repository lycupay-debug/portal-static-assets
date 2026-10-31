package main

// Content write handlers own PATCH versioning and publish validation. They preserve row locks, managed-image reference locks, validation-event insertion, and transaction boundaries.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

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

	if newBody, ok := v["body"].(string); ok {
		oldBody, _ := cur["body"].(string)
		if err := lockContentImageReferences(r.Context(), tx, oldBody, newBody); err != nil {
			if errors.Is(err, errManagedImageNotRegistered) {
				writeError(w, 400, "正文包含不可用的图片引用。", nil)
			} else {
				a.dbError(w, err, "校验正文图片引用失败。")
			}
			return
		}
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
	if err := lockContentImageReferences(r.Context(), tx, body, body); err != nil {
		if errors.Is(err, errManagedImageNotRegistered) {
			writeError(w, 400, "正文包含不可用的图片引用，无法发布。", nil)
		} else {
			a.dbError(w, err, "校验正文图片引用失败。")
		}
		return
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE portal.portal_content SET status='published',published_at=now(),updated_at=now() WHERE content_id=$1 AND author_id=$2`, id, u.ID)
	if err != nil {
		a.dbError(w, err, "发布失败。")
		return
	}
	eid, err := newUUID()
	if err != nil {
		a.dbError(w, err, "生成发布事件标识失败。")
		return
	}
	payload, _ := json.Marshal(map[string]any{"title": title, "slug": slug, "content_type": typ, "tags": tags, "visibility": vis, "summary": summary, "body": body})
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal.event_inbox(event_id,event_type,payload) VALUES($1::uuid,'portal.content.validate',$2::jsonb)`, eid, string(payload)); err != nil {
		a.dbError(w, err, "发布事件登记失败。")
		return
	}
	// Read the response from the same transaction before COMMIT. A post-commit
	// read failure would otherwise return HTTP 500 after the publish and event
	// were already durable, leaving the client unsure whether retrying is safe.
	item, err = oneJSON(r.Context(), tx, `SELECT to_jsonb(x) FROM (SELECT `+contentSelect+` FROM portal.portal_content c WHERE c.content_id=$1 AND c.author_id=$2) x`, id, u.ID)
	if err != nil {
		a.dbError(w, err, "读取发布结果失败，发布事务已取消。")
		return
	}
	if err = tx.Commit(); err != nil {
		a.dbError(w, err, "发布失败。")
		return
	}
	go a.notifyEvent(r.Context(), eid, "portal.content.validate")
	writeJSON(w, 200, map[string]any{"item": item, "eventId": eid})
}
