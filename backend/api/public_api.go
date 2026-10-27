package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

func (a *portalAPI) publicContent(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/public/content" && r.Method == http.MethodGet {
		search := normalizeSearchQuery(r.URL.Query().Get("q"))
		query := `SELECT to_jsonb(x) FROM (SELECT content_id::text AS "id",title,slug,content_type AS "contentType",tags,parent_category AS "parentCategory",child_category AS "childCategory",summary,published_at AS "publishedAt" FROM portal.portal_content WHERE status='published' AND visibility='public' AND deleted_at IS NULL`
		args := []any{}
		if search != "" {
			args = append(args, search)
			query += ` AND (to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(summary, '') || ' ' || coalesce(body, '') || ' ' || coalesce(slug, '') || ' ' || coalesce(portal.text_array_join(tags, ' '), '') || ' ' || coalesce(parent_category, '') || ' ' || coalesce(child_category, '')) @@ websearch_to_tsquery('simple', $1) OR lower(coalesce(title, '') || ' ' || coalesce(summary, '') || ' ' || coalesce(body, '') || ' ' || coalesce(slug, '') || ' ' || coalesce(portal.text_array_join(tags, ' '), '') || ' ' || coalesce(parent_category, '') || ' ' || coalesce(child_category, '')) LIKE '%' || lower($1) || '%' OR regexp_replace(lower(coalesce(title, '') || ' ' || coalesce(summary, '') || ' ' || coalesce(body, '') || ' ' || coalesce(slug, '') || ' ' || coalesce(portal.text_array_join(tags, ' '), '') || ' ' || coalesce(parent_category, '') || ' ' || coalesce(child_category, '')), '[[:space:]]+', '', 'g') LIKE '%' || regexp_replace(lower($1), '[[:space:]]+', '', 'g') || '%' OR lower($1) <% lower(coalesce(title, '') || ' ' || coalesce(summary, '') || ' ' || coalesce(slug, '') || ' ' || coalesce(portal.text_array_join(tags, ' '), '') || ' ' || coalesce(parent_category, '') || ' ' || coalesce(child_category, '')))`
		}
		query += ` ORDER BY published_at DESC NULLS LAST,updated_at DESC LIMIT 100) x`
		cacheKey := "public-content:v1:" + strings.ToLower(search)
		items, err := a.searchCache.get(r.Context(), cacheKey, func() ([]byte, error) {
			rows, err := queryJSONRows(r.Context(), a.db, query, args...)
			if err != nil {
				return nil, err
			}
			return encodeSearchItems(rows)
		})
		if err != nil {
			a.dbError(w, err, "读取公开内容失败。")
			return
		}
		writeCachedItems(w, items)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "public" || parts[1] != "content" {
		writeError(w, 404, "未找到公开内容。", nil)
		return
	}
	slug := parts[2]
	if len(slug) > 100 || !portalSlug.MatchString(slug) {
		writeError(w, 404, "内容不存在或尚未公开。", nil)
		return
	}
	if len(parts) == 4 && parts[3] == "interactions" {
		a.publicInteraction(w, r, slug)
		return
	}
	if len(parts) != 3 || r.Method != http.MethodGet {
		writeError(w, 405, "不支持的请求方式。", nil)
		return
	}
	item, err := oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT c.content_id::text AS "id",c.title,c.slug,c.content_type AS "contentType",c.tags,c.parent_category AS "parentCategory",c.child_category AS "childCategory",c.summary,c.body,c.visibility,c.status,c.created_at AS "createdAt",c.updated_at AS "updatedAt",c.published_at AS "publishedAt",u.display_name AS "authorName",u.created_at AS "authorJoinedAt",(SELECT count(*)::int FROM portal.portal_content_interactions i WHERE i.content_id=c.content_id AND i.interaction_type='like') AS "likeCount",(SELECT count(*)::int FROM portal.portal_content_interactions i WHERE i.content_id=c.content_id AND i.interaction_type='favorite') AS "favoriteCount" FROM portal.portal_content c JOIN portal.portal_users u ON u.user_id=c.author_id WHERE c.slug=$1 AND c.status='published' AND c.visibility='public' AND c.deleted_at IS NULL) x`, slug)
	if err != nil {
		writeError(w, 404, "内容不存在或尚未公开。", nil)
		return
	}
	u, _ := a.session(r)
	likes := []string{}
	if u != nil {
		rows, e := a.db.QueryContext(r.Context(), `SELECT interaction_type FROM portal.portal_content_interactions WHERE content_id=$1 AND user_id=$2`, item["id"], u.ID)
		if e == nil {
			for rows.Next() {
				var v string
				if scanErr := rows.Scan(&v); scanErr != nil {
					e = scanErr
					break
				}
				likes = append(likes, v)
			}
			if e == nil {
				e = rows.Err()
			}
			if closeErr := rows.Close(); closeErr != nil && e == nil {
				e = closeErr
			}
			if e != nil {
				log.Printf("public_content_interactions_read_failed")
			}
		}
	}
	item["viewerLiked"] = contains(likes, "like")
	item["viewerFavorited"] = contains(likes, "favorite")
	writeJSON(w, 200, map[string]any{"item": item})
}
func (a *portalAPI) publicInteraction(w http.ResponseWriter, r *http.Request, slug string) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "不支持的请求方式。", nil)
		return
	}
	u, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	var b struct {
		Type string `json:"type"`
	}
	if err := decodeJSON(w, r, 4096, &b); err != nil {
		writeError(w, 400, "请求格式不正确。", nil)
		return
	}
	if b.Type != "like" && b.Type != "favorite" {
		writeError(w, 400, "互动类型不正确。", nil)
		return
	}
	var id string
	err := a.db.QueryRowContext(r.Context(), `SELECT content_id::text FROM portal.portal_content WHERE slug=$1 AND status='published' AND visibility='public' AND deleted_at IS NULL`, slug).Scan(&id)
	if err != nil {
		writeError(w, 404, "内容不存在或尚未公开。", nil)
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		a.dbError(w, err, "互动操作失败。")
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `DELETE FROM portal.portal_content_interactions WHERE content_id=$1 AND user_id=$2 AND interaction_type=$3`, id, u.ID, b.Type)
	if err != nil {
		a.dbError(w, err, "互动操作失败。")
		return
	}
	n, _ := res.RowsAffected()
	active := n == 0
	if active {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO portal.portal_content_interactions(content_id,user_id,interaction_type) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, id, u.ID, b.Type)
		if err != nil {
			a.dbError(w, err, "互动操作失败。")
			return
		}
	}
	var likes, favs int
	_ = tx.QueryRowContext(r.Context(), `SELECT count(*) FILTER(WHERE interaction_type='like')::int,count(*) FILTER(WHERE interaction_type='favorite')::int FROM portal.portal_content_interactions WHERE content_id=$1`, id).Scan(&likes, &favs)
	if err = tx.Commit(); err != nil {
		a.dbError(w, err, "互动操作失败。")
		return
	}
	writeJSON(w, 200, map[string]any{"active": active, "likeCount": likes, "favoriteCount": favs})
}

func (a *portalAPI) publicResources(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/public/resources"), "/")
	if path == "" && r.Method == http.MethodGet {
		search := normalizeSearchQuery(r.URL.Query().Get("q"))
		query := `SELECT to_jsonb(x) FROM (SELECT resource_id::text AS "id",title,description,resource_type AS "type",grade AS "grade",is_public AS "isPublic",resource_url AS "url",CASE WHEN stored_filename IS NOT NULL THEN '/api/public/resources/' || resource_id::text || '/download' ELSE NULL END AS "downloadUrl",original_filename AS "filename",mime_type AS "mimeType",file_size AS "fileSize",created_at AS "createdAt",updated_at AS "updatedAt" FROM portal.teaching_resources WHERE is_public=TRUE AND deleted_at IS NULL`
		args := []any{}
		orderBy := "updated_at DESC"
		if search != "" {
			args = append(args, search)
			orderBy = resourceSearchOrderSQL("$1") + ", " + resourceSearchSimilarityOrderSQL("", "$1") + ", updated_at DESC"
			query += ` AND (` + resourceDirectSearchMatchSQL("", "$1") + ` OR (NOT EXISTS (SELECT 1 FROM portal.teaching_resources matched WHERE matched.is_public=TRUE AND matched.deleted_at IS NULL AND ` + resourceDirectSearchMatchSQL("matched", "$1") + `) AND ` + resourceFuzzySearchMatchSQL("", "$1") + `))`
		}
		query += ` ORDER BY ` + orderBy + ` LIMIT 500) x`
		cacheKey := "public-resources:v2:" + strings.ToLower(search)
		items, err := a.searchCache.get(r.Context(), cacheKey, func() ([]byte, error) {
			rows, err := queryJSONRows(r.Context(), a.db, query, args...)
			if err != nil {
				return nil, err
			}
			return encodeSearchItems(rows)
		})
		if err != nil {
			a.dbError(w, err, "读取公开教育资源失败。")
			return
		}
		writeCachedItems(w, items)
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) == 2 && portalUUID.MatchString(parts[0]) && parts[1] == "preview" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		a.serveResourceMP3Preview(w, r, parts[0], true, "")
		return
	}
	if len(parts) == 2 && portalUUID.MatchString(parts[0]) && parts[1] == "download" && r.Method == http.MethodGet {
		var stored, filename, mtype sql.NullString
		err := a.db.QueryRowContext(r.Context(), `SELECT stored_filename,original_filename,mime_type FROM portal.teaching_resources WHERE resource_id=$1 AND is_public=TRUE AND deleted_at IS NULL`, parts[0]).Scan(&stored, &filename, &mtype)
		if err != nil || !stored.Valid {
			writeError(w, 404, "公开资源不存在。", nil)
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
	writeError(w, 404, "未找到公开资源接口。", nil)
}
