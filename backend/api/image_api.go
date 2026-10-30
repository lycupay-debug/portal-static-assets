package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func (a *portalAPI) uploadImage(w http.ResponseWriter, r *http.Request) {
	u, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	var b struct {
		MIME string `json:"mime"`
		Data string `json:"data"`
	}
	if err := decodeJSON(w, r, 12*1024*1024, &b); err != nil {
		writeError(w, 400, "请求格式不正确。", nil)
		return
	}
	mime := strings.ToLower(strings.TrimSpace(b.MIME))
	allowed := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp", "image/avif": "avif"}
	ext, ok := allowed[mime]
	if !ok {
		writeError(w, 400, "仅支持 PNG、JPEG、GIF、WebP 或 AVIF 图片。", nil)
		return
	}
	data, err := base64.StdEncoding.DecodeString(b.Data)
	if err != nil || len(data) == 0 {
		writeError(w, 400, "上传图片数据无效。", nil)
		return
	}
	if len(data) > 8*1024*1024 {
		writeError(w, 413, "图片大小不能超过 8 MB。", nil)
		return
	}
	if !validImageSignature(mime, data) {
		writeError(w, 400, "图片文件内容与声明格式不匹配。", nil)
		return
	}
	if err = os.MkdirAll(a.uploadDir, 0750); err != nil {
		writeError(w, 500, "图片保存失败。", nil)
		return
	}
	id, err := newUUID()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "图片文件标识生成失败，请稍后重试。", nil)
		return
	}
	name := id + "." + ext
	if err = writeManagedFile(a.uploadDir, name, data, 0640); err != nil {
		writeError(w, 500, "图片保存失败。", nil)
		return
	}
	sum := sha256.Sum256(data)
	_, err = a.db.ExecContext(r.Context(), `INSERT INTO portal.uploaded_images(filename,uploader_id,mime_type,sha256,file_size_bytes) VALUES($1,$2::uuid,$3,$4,$5)`, name, u.ID, mime, hex.EncodeToString(sum[:]), len(data))
	if err != nil {
		_ = removeManagedFile(a.uploadDir, name)
		a.dbError(w, err, "图片登记失败。")
		return
	}
	writeJSON(w, 201, map[string]any{"url": "/api/uploads/" + name, "mime": mime, "size": len(data), "public": false})
}

func (a *portalAPI) serveUploadedImage(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/uploads/")
	if !uploadedImageName.MatchString(name) || strings.Contains(name, "/") || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	var uploaderID, mime string
	if err := a.db.QueryRowContext(r.Context(), `SELECT uploader_id::text,mime_type FROM portal.uploaded_images WHERE filename=$1`, name).Scan(&uploaderID, &mime); err != nil {
		http.NotFound(w, r)
		return
	}
	var referenced bool
	if err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM portal.portal_content WHERE status='published' AND visibility='public' AND deleted_at IS NULL AND position($1 in body)>0)`, "/uploads/"+name).Scan(&referenced); err != nil {
		writeError(w, 503, "图片访问状态暂时无法核验。", nil)
		return
	}
	if !referenced {
		u, err := a.session(r)
		if err != nil {
			writeError(w, 503, "图片访问状态暂时无法核验。", nil)
			return
		}
		if u == nil || (u.ID != uploaderID && !u.IsAdmin) {
			http.NotFound(w, r)
			return
		}
	}
	f, err := openManagedFile(a.uploadDir, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

func validImageSignature(m string, b []byte) bool {
	switch m {
	case "image/png":
		return len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n"
	case "image/jpeg":
		return len(b) >= 3 && b[0] == 0xff && b[1] == 0xd8 && b[2] == 0xff
	case "image/gif":
		return len(b) >= 6 && (string(b[:6]) == "GIF87a" || string(b[:6]) == "GIF89a")
	case "image/webp":
		return len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP"
	case "image/avif":
		return len(b) >= 12 && strings.Contains(string(b[4:12]), "avif")
	}
	return false
}
