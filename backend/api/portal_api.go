package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	portalCookie      = "portal_session"
	portalSessionDays = 30
	maxContentBody    = 512 * 1024
	maxResourceBody   = 36 * 1024 * 1024
	maxResourceFile   = 25 * 1024 * 1024
)

var (
	portalUUID                = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	uploadedImageName         = regexp.MustCompile(`^[0-9a-f-]{36}\.(png|jpg|gif|webp|avif)$`)
	uploadedImageURLPattern   = regexp.MustCompile(`/uploads/([0-9a-f-]{36}\.(?:png|jpg|gif|webp|avif))`)
	externalContentURLPattern = regexp.MustCompile(`https?://[^[:space:]<>\"')]+`)
	externalImageFilePattern  = regexp.MustCompile(`(?i)\.(?:png|jpe?g|webp|gif|svg|avif)(?:\?|$)`)
	portalSlug                = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	portalEmail               = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	portalTypes               = map[string]bool{"article": true, "tool": true, "resource": true, "project": true}
	portalTags                = map[string]bool{
		"教育": true, "学前教育": true, "小学教育": true, "中学教育": true, "高等教育": true, "职业教育": true, "家庭教育": true, "教学资源": true,
		"教育人力资源": true, "事业编": true, "国企": true, "省考": true, "国考": true,
		"旅游": true, "住宿": true, "交通": true, "景点攻略": true, "旅行规划": true, "美食体验": true, "网络": true, "网络安全": true, "网络维护": true, "网络基础知识": true, "网络诊断": true, "云与服务器": true,
		"系统": true, "操作系统": true, "系统配置": true, "系统维护": true, "部署运维": true, "故障排查": true, "编程": true, "前端开发": true, "后端开发": true, "Python": true, "JavaScript / TypeScript": true, "数据库": true, "API 与集成": true, "代码实践": true, "软件工具": true, "效率工具": true, "开发工具": true, "文档工具": true, "AI 工具": true, "自动化工具": true,
	}
	portalTaxonomy = map[string][]string{
		"教育":     {"学前教育", "小学教育", "中学教育", "高等教育", "职业教育", "家庭教育", "教学资源"},
		"年级":     {"一年级", "二年级", "三年级", "四年级", "五年级", "六年级", "初一", "初二", "初三", "高一", "高二", "高三", "本科", "硕士"},
		"教育人力资源": {"事业编", "国企", "省考", "国考"},
		"旅游":     {"住宿", "交通", "景点攻略", "旅行规划", "美食体验"},
		"网络":     {"网络安全", "网络维护", "网络基础知识", "网络诊断", "云与服务器"},
		"系统":     {"操作系统", "系统配置", "系统维护", "部署运维", "故障排查"},
		"编程":     {"前端开发", "后端开发", "Python", "JavaScript / TypeScript", "数据库", "API 与集成", "代码实践"},
		"软件工具":   {"效率工具", "开发工具", "文档工具", "AI 工具", "自动化工具"},
	}
	contentColumns = map[string]string{"title": "title", "slug": "slug", "contentType": "content_type", "tags": "tags", "parentCategory": "parent_category", "childCategory": "child_category", "summary": "summary", "body": "body", "visibility": "visibility"}
)

type portalAPI struct {
	db          *sql.DB
	ipc         IPCClient
	uploadDir   string
	resourceDir string
	searchCache *searchResultCache
	mu          sync.Mutex
	rate        map[string]rateEntry
}
type rateEntry struct {
	count int
	until time.Time
}
type portalUser struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"displayName"`
	CreatedAt   time.Time  `json:"createdAt"`
	IsAdmin     bool       `json:"isAdmin"`
	ExpiresAt   *time.Time `json:"-"`
}
type portalContentInput struct {
	Title             *string   `json:"title"`
	Slug              *string   `json:"slug"`
	ContentType       *string   `json:"contentType"`
	Tags              *[]string `json:"tags"`
	ParentCategory    *string   `json:"parentCategory"`
	ChildCategory     *string   `json:"childCategory"`
	Summary           *string   `json:"summary"`
	Body              *string   `json:"body"`
	Visibility        *string   `json:"visibility"`
	ParentCategorySet bool      `json:"-"`
	ChildCategorySet  bool      `json:"-"`
}

func (in *portalContentInput) UnmarshalJSON(data []byte) error {
	type alias portalContentInput
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*in = portalContentInput(decoded)
	_, in.ParentCategorySet = fields["parentCategory"]
	_, in.ChildCategorySet = fields["childCategory"]
	return nil
}

type resourceInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Grade       string `json:"grade"`
	IsPublic    *bool  `json:"isPublic"`
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	Data        string `json:"data"`
}

func newPortalAPI(db *sql.DB, ipc IPCClient) *portalAPI {
	uploadDir := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_UPLOAD_DIR"))
	if uploadDir == "" {
		uploadDir = "/usr/local/www/personal-portal/uploads"
	}
	resourceDir := strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_RESOURCE_DIR"))
	if resourceDir == "" {
		resourceDir = "/var/db/personal-portal/resources"
	}
	return &portalAPI{db: db, ipc: ipc, uploadDir: uploadDir, resourceDir: resourceDir, searchCache: newSearchResultCache(), rate: make(map[string]rateEntry)}
}
func (a *portalAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Any content/resource write may change indexed public search results.
	// Invalidate even on a failed write: this only causes a harmless cache miss.
	if r.Method != http.MethodGet && r.Method != http.MethodHead && (strings.HasPrefix(r.URL.Path, "/content") || strings.HasPrefix(r.URL.Path, "/resources")) {
		defer a.searchCache.invalidate()
	}
	if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
		writeJSON(w, 200, map[string]any{"status": "ok"})
		return
	}
	if r.URL.Path == "/layout-policy" && r.Method == http.MethodGet {
		writeJSON(w, 200, layoutPolicy(r.URL.Query()))
		return
	}
	if r.URL.Path == "/uploads" && r.Method == http.MethodPost {
		a.uploadImage(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/uploads/") && r.Method == http.MethodGet {
		a.serveUploadedImage(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/auth/") {
		a.auth(w, r)
		return
	}
	if r.URL.Path == "/public/legal" || strings.HasPrefix(r.URL.Path, "/public/legal/") {
		a.publicLegal(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/admin/") {
		a.rbac(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/compliance/") {
		a.compliance(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/system-ops/") {
		a.systemOps(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/public/content") {
		a.publicContent(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/public/resources") {
		a.publicResources(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/content") {
		a.content(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/resources") {
		a.resources(w, r)
		return
	}
	writeError(w, 404, "未找到接口。", nil)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, message string, errorsList []string) {
	body := map[string]any{"error": message}
	if errorsList != nil {
		body["errors"] = errorsList
	}
	writeJSON(w, status, body)
}
func decodeJSON(w http.ResponseWriter, r *http.Request, max int64, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("request must contain one JSON value")
	}
	return nil
}
func runeLen(s string) int { return utf8.RuneCountInString(s) }
func rateLimitClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	remoteIP := net.ParseIP(strings.Trim(host, "[]"))
	if remoteIP != nil && remoteIP.IsLoopback() {
		// Trust only Caddy's private header after it validates the peer against
		// Cloudflare's trusted-proxy ranges. Direct loopback callers can forge
		// X-Forwarded-For, so that generic header is intentionally ignored.
		forwarded := strings.TrimSpace(r.Header.Get("X-Portal-Client-IP"))
		if forwardedIP := net.ParseIP(forwarded); forwardedIP != nil {
			return forwardedIP.String()
		}
	}
	if remoteIP != nil {
		return remoteIP.String()
	}
	if host == "" {
		return "unknown"
	}
	return host
}

func (a *portalAPI) rateLimited(w http.ResponseWriter, r *http.Request) bool {
	key := rateLimitClientIP(r) + ":" + r.URL.Path
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	v := a.rate[key]
	if !now.Before(v.until) {
		v = rateEntry{until: now.Add(15 * time.Minute)}
	}
	v.count++
	a.rate[key] = v
	if len(a.rate) > 5000 {
		for k, e := range a.rate {
			if now.After(e.until) {
				delete(a.rate, k)
			}
		}
	}
	if v.count > 12 {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(time.Until(v.until).Seconds()))))
		writeError(w, 429, "操作过于频繁，请稍后再试。", nil)
		return true
	}
	return false
}
func layoutPolicy(q url.Values) map[string]any {
	width := readDimension(q.Get("width"), 1440, 10000)
	height := readDimension(q.Get("height"), 900, 10000)
	profile, gx, cg, side := "desktop", "clamp(2.5vw, 4.2vw, 6.5vw)", "clamp(2vw, 3vw, 4.5vw)", "18vw"
	switch {
	case width <= 380:
		profile, gx, cg, side = "compact-mobile", "3vw", "3vw", "100%"
	case width <= 760:
		profile, gx, cg, side = "mobile", "4vw", "4vw", "100%"
	case width <= 1100:
		profile, gx, cg, side = "tablet", "3.2vw", "2.6vw", "21vw"
	case width >= 1920:
		profile, gx, cg, side = "ultrawide", "4vw", "2.5vw", "15vw"
	}
	gy := "clamp(2vh, 4vh, 5.5vh)"
	if height <= 600 {
		gy = "2vh"
	} else if width <= 760 {
		gy = "3vh"
	}
	return map[string]any{"version": 1, "viewport": map[string]int{"width": width, "height": height}, "profile": profile, "cssVariables": map[string]string{"--layout-gutter-x": gx, "--layout-gutter-y": gy, "--layout-content-gutter": cg, "--layout-sidebar-track": side, "--layout-table-top-gap": "8vh"}}
}
func readDimension(s string, fallback, maximum int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return fallback
	}
	if n < 240 {
		n = 240
	}
	if n > maximum {
		n = maximum
	}
	return n
}

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
	name := randomUUID() + "." + ext
	fullPath := filepath.Join(a.uploadDir, name)
	if err = os.WriteFile(fullPath, data, 0640); err != nil {
		writeError(w, 500, "图片保存失败。", nil)
		return
	}
	sum := sha256.Sum256(data)
	_, err = a.db.ExecContext(r.Context(), `INSERT INTO portal.uploaded_images(filename,uploader_id,mime_type,sha256,file_size_bytes) VALUES($1,$2::uuid,$3,$4,$5)`, name, u.ID, mime, hex.EncodeToString(sum[:]), len(data))
	if err != nil {
		_ = os.Remove(fullPath)
		a.dbError(w, err, "图片登记失败。")
		return
	}
	writeJSON(w, 201, map[string]any{"url": "/uploads/" + name, "mime": mime, "size": len(data), "public": false})
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
	f, err := os.Open(filepath.Join(a.uploadDir, name))
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
func randomUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b)
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}

const contentSelect = `content_id::text AS "id",author_id::text AS "authorId",title,slug,content_type AS "contentType",tags,parent_category AS "parentCategory",child_category AS "childCategory",summary,body,visibility,status,created_at AS "createdAt",updated_at AS "updatedAt",published_at AS "publishedAt",deleted_at AS "deletedAt"`

func queryJSON(ctx context.Context, db interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item map[string]any
		if err = json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func queryJSONRows(ctx context.Context, db *sql.DB, query string, args ...any) ([]map[string]any, error) {
	return queryJSON(ctx, db, query, args...)
}
func oneJSON(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, query string, args ...any) (map[string]any, error) {
	var raw []byte
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
		return nil, err
	}
	var item map[string]any
	err := json.Unmarshal(raw, &item)
	return item, err
}
func contentJSONSelect() string { return "to_jsonb(c)" }
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
func eventMetricsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		rows, err := db.QueryContext(ctx, `SELECT status, count(*)::int FROM portal.event_inbox GROUP BY status`)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "metrics database query failed", nil)
			return
		}
		defer rows.Close()
		counts := map[string]int{"received": 0, "processing": 0, "completed": 0, "failed": 0}
		for rows.Next() {
			var status string
			var count int
			if err := rows.Scan(&status, &count); err != nil {
				writeError(w, http.StatusServiceUnavailable, "metrics database query failed", nil)
				return
			}
			counts[status] = count
		}
		if err := rows.Err(); err != nil {
			writeError(w, http.StatusServiceUnavailable, "metrics database query failed", nil)
			return
		}
		var dueDispatch, expiredProcessing int
		var oldestReceived sql.NullFloat64
		if err := db.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE dispatch_state='pending' AND next_dispatch_at<=now())::int, count(*) FILTER(WHERE status='processing' AND processing_lease_until<now())::int, EXTRACT(EPOCH FROM now()-min(received_at) FILTER (WHERE status='received')) FROM portal.event_inbox`).Scan(&dueDispatch, &expiredProcessing, &oldestReceived); err != nil {
			writeError(w, http.StatusServiceUnavailable, "metrics database query failed", nil)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprintln(w, "# HELP personal_portal_events Current event inbox rows by business status.")
		fmt.Fprintln(w, "# TYPE personal_portal_events gauge")
		for _, status := range []string{"received", "processing", "completed", "failed"} {
			fmt.Fprintf(w, "personal_portal_events{status=%q} %d\n", status, counts[status])
		}
		var failedFileCleanupEvents int
		if err := db.QueryRowContext(ctx, `SELECT count(*)::int FROM portal.event_inbox WHERE event_type=$1 AND status='failed'`, fileCleanupEventType).Scan(&failedFileCleanupEvents); err != nil {
			writeError(w, http.StatusServiceUnavailable, "metrics database query failed", nil)
			return
		}
		fmt.Fprintln(w, "# HELP personal_portal_file_cleanup_events_failed File cleanup events exhausted retries and require inspection.")
		fmt.Fprintln(w, "# TYPE personal_portal_file_cleanup_events_failed gauge")
		fmt.Fprintf(w, "personal_portal_file_cleanup_events_failed %d\n", failedFileCleanupEvents)
		fmt.Fprintln(w, "# HELP personal_portal_event_dispatch_due Events waiting for a dispatch attempt.")
		fmt.Fprintln(w, "# TYPE personal_portal_event_dispatch_due gauge")
		fmt.Fprintf(w, "personal_portal_event_dispatch_due %d\n", dueDispatch)
		fmt.Fprintln(w, "# HELP personal_portal_event_processing_leases_expired Processing leases that have expired and can be recovered.")
		fmt.Fprintln(w, "# TYPE personal_portal_event_processing_leases_expired gauge")
		fmt.Fprintf(w, "personal_portal_event_processing_leases_expired %d\n", expiredProcessing)
		fmt.Fprintln(w, "# HELP personal_portal_event_oldest_received_age_seconds Age of the oldest received event; zero if none exist.")
		fmt.Fprintln(w, "# TYPE personal_portal_event_oldest_received_age_seconds gauge")
		if oldestReceived.Valid {
			fmt.Fprintf(w, "personal_portal_event_oldest_received_age_seconds %.3f\n", oldestReceived.Float64)
		} else {
			fmt.Fprintln(w, "personal_portal_event_oldest_received_age_seconds 0")
		}
	}
}
