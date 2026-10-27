package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func (a *portalAPI) serveResourceMP3Preview(w http.ResponseWriter, r *http.Request, id string, public bool, ownerID string) {
	var stored, filename sql.NullString
	var err error
	if public {
		err = a.db.QueryRowContext(r.Context(), `SELECT stored_filename,original_filename FROM portal.teaching_resources WHERE resource_id=$1 AND is_public=TRUE AND deleted_at IS NULL AND lower(original_filename) LIKE '%.mp3'`, id).Scan(&stored, &filename)
	} else {
		err = a.db.QueryRowContext(r.Context(), `SELECT stored_filename,original_filename FROM portal.teaching_resources WHERE resource_id=$1 AND owner_id=$2 AND deleted_at IS NULL AND lower(original_filename) LIKE '%.mp3'`, id, ownerID).Scan(&stored, &filename)
	}
	if err != nil || !stored.Valid || !filename.Valid {
		writeError(w, http.StatusNotFound, "MP3 资源不存在或无权访问。", nil)
		return
	}
	filePath := filepath.Join(a.resourceDir, filepath.Base(stored.String))
	f, err := os.Open(filePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "资源文件缺失。", nil)
		return
	}
	defer f.Close()
	serveMP3Preview(w, r, f, filename.String)
}

// serveMP3Preview uses net/http's range-aware file serving so native browser
// audio controls can seek without downloading the entire file first.
func serveMP3Preview(w http.ResponseWriter, r *http.Request, f *os.File, filename string) {
	info, err := f.Stat()
	if err != nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename*=UTF-8''%s", url.PathEscape(filename)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, filepath.Base(filename), info.ModTime(), f)
}

func (a *portalAPI) resources(w http.ResponseWriter, r *http.Request) {
	u, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	path := strings.Trim(r.URL.Path, "/")
	if path == "resources" && r.Method == http.MethodGet {
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
	if path == "resources/trash" && r.Method == http.MethodGet {
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
	if path == "resources" && r.Method == http.MethodPost {
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
		err = a.db.QueryRowContext(r.Context(), `INSERT INTO portal.teaching_resources(owner_id,title,description,resource_type,grade,is_public,resource_url,original_filename,stored_filename,mime_type,file_size) VALUES($1,$2,$3,$4,$5,COALESCE($6,FALSE),$7,$8,$9,$10,$11) RETURNING resource_id::text`, u.ID, strings.TrimSpace(b.Title), strings.TrimSpace(b.Description), resourceType(b.Type), nullableString(resourceGrade(b.Grade)), b.IsPublic, urlAny, filenameAny, storedAny, mimeAny, fileSize).Scan(&resourceID)
		if err != nil {
			if stored != "" {
				_ = os.Remove(filepath.Join(a.resourceDir, stored))
			}
			a.dbError(w, err, "资源保存失败。")
			return
		}
		item, err := oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT resource_id::text AS "id",title,description,resource_type AS "type",grade AS "grade",is_public AS "isPublic",resource_url AS "url",original_filename AS "filename",mime_type AS "mimeType",file_size AS "fileSize",created_at AS "createdAt",updated_at AS "updatedAt" FROM portal.teaching_resources WHERE resource_id=$1) x`, resourceID)
		if err != nil {
			a.dbError(w, err, "资源已保存，但读取结果失败。")
			return
		}
		writeJSON(w, 201, map[string]any{"item": item})
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 || !portalUUID.MatchString(parts[1]) {
		writeError(w, 404, "未找到资源接口。", nil)
		return
	}
	id := parts[1]
	action := ""
	if len(parts) > 2 {
		action = parts[2]
	}
	if action == "preview" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		a.serveResourceMP3Preview(w, r, id, false, u.ID)
		return
	}
	if action == "download" && r.Method == http.MethodGet {
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
		path := filepath.Join(a.resourceDir, filepath.Base(stored.String))
		f, err := os.Open(path)
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
	if action == "restore" && r.Method == http.MethodPost {
		res, err := a.db.ExecContext(r.Context(), `UPDATE portal.teaching_resources SET deleted_at=NULL,updated_at=now() WHERE resource_id=$1 AND owner_id=$2 AND deleted_at IS NOT NULL`, id, u.ID)
		if err != nil {
			a.dbError(w, err, "资源恢复失败。")
			return
		}
		n, _ := res.RowsAffected()
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
	if action == "permanent" && r.Method == http.MethodDelete {
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
			go a.notifyEvent(cleanupEventID, fileCleanupEventType)
		}
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	if action != "" {
		writeError(w, 404, "未找到资源接口。", nil)
		return
	}
	if r.Method == http.MethodDelete {
		res, err := a.db.ExecContext(r.Context(), `UPDATE portal.teaching_resources SET deleted_at=now(),updated_at=now() WHERE resource_id=$1 AND owner_id=$2 AND deleted_at IS NULL`, id, u.ID)
		if err != nil {
			a.dbError(w, err, "删除资源失败。")
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			writeError(w, 404, "资源不存在或已在回收站。", nil)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	if r.Method == http.MethodPatch {
		a.patchResource(w, r, u, id)
		return
	}
	writeError(w, 405, "不支持的请求方式。", nil)
}
func resourceGrade(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, grade := range portalTaxonomy["年级"] {
		if s == grade {
			return s
		}
	}
	return "__invalid_grade__"
}

func resourceType(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "其他"
	}
	return s
}
func validateResourceFile(b resourceInput) ([]byte, string, string, error) {
	title := strings.TrimSpace(b.Title)
	desc := strings.TrimSpace(b.Description)
	typ := resourceType(b.Type)
	grade := resourceGrade(b.Grade)
	if title == "" || runeLen(title) > 200 || runeLen(desc) > 2000 || runeLen(typ) > 50 || grade == "__invalid_grade__" {
		return nil, "", "", errors.New("请填写有效的资源名称、类型、年级和资源说明。")
	}
	if strings.TrimSpace(b.URL) != "" && (!strings.HasPrefix(strings.ToLower(strings.TrimSpace(b.URL)), "http://") && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(b.URL)), "https://") || len(strings.TrimSpace(b.URL)) > 2048) {
		return nil, "", "", errors.New("资源 URL 必须是有效的 HTTP/HTTPS 链接，且不超过 2048 个字符。")
	}
	if b.Data == "" {
		if strings.TrimSpace(b.URL) == "" {
			return nil, "", "", errors.New("请上传文件或填写 URL 链接。")
		}
		return nil, "", "", nil
	}
	name := filepath.Base(strings.ReplaceAll(b.Filename, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r == 0 || r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	if name == "" || len(name) > 255 {
		return nil, "", "", errors.New("上传文件数据无效。")
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	mimes := map[string]string{"ppt": "application/vnd.ms-powerpoint", "pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation", "mp3": "audio/mpeg", "pdf": "application/pdf", "doc": "application/msword", "docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "txt": "text/plain"}
	mt, ok := mimes[ext]
	if !ok {
		return nil, "", "", errors.New("支持的格式：PPT、PPTX、MP3、PDF、DOC、DOCX、TXT。")
	}
	data, err := base64.StdEncoding.DecodeString(b.Data)
	if err != nil || len(data) == 0 {
		return nil, "", "", errors.New("上传文件数据无效。")
	}
	if len(data) > maxResourceFile {
		return data, name, mt, nil
	}
	valid := false
	switch ext {
	case "ppt", "doc":
		valid = len(data) >= 8 && hex.EncodeToString(data[:4]) == "d0cf11e0"
	case "pptx", "docx":
		valid = len(data) >= 4 && hex.EncodeToString(data[:4]) == "504b0304"
	case "mp3":
		valid = len(data) >= 3 && (string(data[:3]) == "ID3" || (len(data) > 1 && data[0] == 0xff && (data[1]&0xe0) == 0xe0))
	case "pdf":
		valid = len(data) >= 5 && string(data[:5]) == "%PDF-"
	case "txt":
		valid = !strings.ContainsRune(string(data[:min(len(data), 8192)]), '\x00')
	}
	if !valid {
		return nil, "", "", errors.New("文件内容与扩展名不匹配，上传已拒绝。")
	}
	return data, name, mt, nil
}
func (a *portalAPI) storeResourceFile(data []byte, filename string) (string, error) {
	if len(data) == 0 {
		return "", nil
	}
	ext := strings.ToLower(filepath.Ext(filename))
	name := randomUUID() + ext
	if err := os.MkdirAll(a.resourceDir, 0750); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(a.resourceDir, name), data, 0640); err != nil {
		return "", err
	}
	return name, nil
}
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
	var old sql.NullString
	var currentTitle, currentDescription, currentType string
	var currentFilename, currentMime sql.NullString
	var currentGrade, currentURL sql.NullString
	var currentPublic bool
	var currentSize int64
	err := a.db.QueryRowContext(r.Context(), "SELECT stored_filename,title,description,resource_type,grade,resource_url,is_public,original_filename,mime_type,coalesce(file_size,0) FROM portal.teaching_resources WHERE resource_id=$1 AND owner_id=$2 AND deleted_at IS NULL", id, u.ID).Scan(&old, &currentTitle, &currentDescription, &currentType, &currentGrade, &currentURL, &currentPublic, &currentFilename, &currentMime, &currentSize)
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
	if len(data) > 0 {
		if old.Valid && old.String != "" && name == currentFilename.String {
			currentData, readErr := os.ReadFile(filepath.Join(a.resourceDir, filepath.Base(old.String)))
			if readErr != nil {
				a.dbError(w, readErr, "读取现有资源文件失败，未保存任何修改。")
				return
			}
			oldHash, newHash := sha256.Sum256(currentData), sha256.Sum256(data)
			if oldHash == newHash {
				data = nil
				name, mt = "", ""
			}
		}
	}
	stored := old.String
	var newStored string
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
	res, err := a.db.ExecContext(r.Context(), `UPDATE portal.teaching_resources SET title=$3,description=$4,resource_type=$5,grade=$6,is_public=COALESCE($7,is_public),resource_url=$8,original_filename=COALESCE($9,original_filename),stored_filename=$10,mime_type=COALESCE($11,mime_type),file_size=COALESCE($12,file_size),updated_at=now() WHERE resource_id=$1 AND owner_id=$2 AND deleted_at IS NULL`, id, u.ID, b.Title, b.Description, b.Type, nullableString(b.Grade), b.IsPublic, urlValue, fileNameValue, nullableString(stored), mimeValue, sizeValue)
	if err != nil {
		if newStored != "" {
			_ = os.Remove(filepath.Join(a.resourceDir, newStored))
		}
		a.dbError(w, err, "资源更新失败。")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeError(w, 404, "资源不存在或无权编辑。", nil)
		return
	}
	if newStored != "" && old.Valid {
		_ = os.Remove(filepath.Join(a.resourceDir, filepath.Base(old.String)))
	}
	item, err := oneJSON(r.Context(), a.db, `SELECT to_jsonb(x) FROM (SELECT resource_id::text AS "id",title,description,resource_type AS "type",grade AS "grade",is_public AS "isPublic",resource_url AS "url",original_filename AS "filename",mime_type AS "mimeType",file_size AS "fileSize",created_at AS "createdAt",updated_at AS "updatedAt" FROM portal.teaching_resources WHERE resource_id=$1) x`, id)
	if err != nil {
		a.dbError(w, err, "资源已更新，但读取结果失败。")
		return
	}
	writeJSON(w, 200, map[string]any{"item": item})
}
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// eventMetricsHandler exposes database-derived Prometheus gauges. It is an
// internal endpoint; the public Caddy gateway explicitly blocks /api/metrics.
