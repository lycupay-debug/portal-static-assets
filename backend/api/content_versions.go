package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
)

func saveContentVersion(ctx context.Context, tx *sql.Tx, item map[string]any, userID string) error {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_number),0)+1 FROM portal.portal_content_versions WHERE content_id=$1`, item["id"]).Scan(&n)
	if err != nil {
		return err
	}
	snap := map[string]any{}
	for _, k := range []string{"id", "title", "slug", "contentType", "tags", "parentCategory", "childCategory", "summary", "body", "visibility", "status", "publishedAt", "createdAt", "updatedAt"} {
		if v, ok := item[k]; ok {
			snap[k] = v
		}
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO portal.portal_content_versions(content_id,author_id,version_number,snapshot) VALUES($1,$2,$3,$4::jsonb)`, item["id"], userID, n, string(raw))
	return err
}
func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func (a *portalAPI) restoreVersion(w http.ResponseWriter, r *http.Request, u *portalUser, id, versionID string) {
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dbError(w, err, "恢复版本失败。")
		return
	}
	defer tx.Rollback()
	cur, err := oneJSON(r.Context(), tx, `SELECT to_jsonb(x) FROM (SELECT `+contentSelect+` FROM portal.portal_content c WHERE c.content_id=$1 AND c.author_id=$2 AND c.deleted_at IS NULL FOR UPDATE) x`, id, u.ID)
	if err != nil {
		writeError(w, 404, "内容不存在或无权访问。", nil)
		return
	}

	var snapshotRaw []byte
	err = tx.QueryRowContext(r.Context(), `SELECT snapshot FROM portal.portal_content_versions WHERE version_id=$1 AND content_id=$2 AND author_id=$3`, versionID, id, u.ID).Scan(&snapshotRaw)
	if err != nil {
		writeError(w, 404, "历史版本不存在。", nil)
		return
	}
	var snap map[string]any
	if err = json.Unmarshal(snapshotRaw, &snap); err != nil {
		writeError(w, 400, "历史版本数据未通过校验，无法恢复。", nil)
		return
	}
	if err = saveContentVersion(r.Context(), tx, cur, u.ID); err != nil {
		a.dbError(w, err, "恢复版本失败。")
		return
	}
	tags := stringSlice(snap["tags"])
	vis, _ := snap["visibility"].(string)
	if cur["status"] == "published" {
		vis = "public"
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE portal.portal_content SET title=$3,slug=$4,content_type=$5,tags=$6,parent_category=$7,child_category=$8,summary=$9,body=$10,visibility=$11,updated_at=now() WHERE content_id=$1 AND author_id=$2`, id, u.ID, snap["title"], snap["slug"], snap["contentType"], tags, snap["parentCategory"], snap["childCategory"], snap["summary"], snap["body"], vis)
	if err != nil {
		a.dbError(w, err, "恢复版本失败。")
		return
	}
	var eventID any
	if cur["status"] == "published" {
		eid := randomUUID()
		payload, _ := json.Marshal(map[string]any{"title": snap["title"], "slug": snap["slug"], "content_type": snap["contentType"], "tags": tags, "visibility": vis, "summary": snap["summary"], "body": snap["body"]})
		_, err = tx.ExecContext(r.Context(), `INSERT INTO portal.event_inbox(event_id,event_type,payload) VALUES($1::uuid,'portal.content.validate',$2::jsonb)`, eid, string(payload))
		if err != nil {
			a.dbError(w, err, "恢复版本事件登记失败。")
			return
		}
		eventID = eid
	}
	if err = tx.Commit(); err != nil {
		a.dbError(w, err, "恢复版本失败。")
		return
	}
	if eid, ok := eventID.(string); ok {
		go a.notifyEvent(r.Context(), eid, "portal.content.validate")
	}
	item, err := oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT `+contentSelect+` FROM portal.portal_content c WHERE c.content_id=$1) x`, id)
	if err != nil {
		a.dbError(w, err, "读取恢复结果失败。")
		return
	}
	writeJSON(w, 200, map[string]any{"item": item, "eventId": eventID})
}
func stringSlice(v any) []string {
	out := []string{}
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		for _, i := range x {
			if s, ok := i.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}
