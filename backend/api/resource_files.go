package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
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
	f, err := openManagedFile(a.resourceDir, stored.String)
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
	if err := writeManagedFile(a.resourceDir, name, data, 0640); err != nil {
		return "", err
	}
	return name, nil
}
