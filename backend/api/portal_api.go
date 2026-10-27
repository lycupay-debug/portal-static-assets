package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
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
