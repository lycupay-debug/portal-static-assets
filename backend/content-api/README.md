# Content API

内容管理是独立的业务模块，由现有 Node.js API 进程挂载路由，复用认证会话、数据库连接池、JSON 响应和请求体解析，不额外启动第二个服务。

## Responsibilities

- `handlers.mjs`: 内容业务规则、输入校验、内容 CRUD、发布状态与公开读取。
- `backend/auth-api/server.mjs`: HTTP 服务、会话认证、共享数据库连接和通用响应能力。
- `backend/migrations/007_portal_content.sql`: 内容表、约束、索引和迁移登记。
- `backend/migrations/008_portal_content_versions.sql`: 内容历史快照表、回收站索引和迁移登记。
- `backend/api`: Go 事件接收、分发、处理。公开内容发布时，内容更新与 `portal.content.validate` 事件在同一 PostgreSQL 事务中提交。

## Endpoints

这些路径经过 Caddy 的 `/api/*` 前缀代理后，由 Node.js 收到去掉 `/api` 的路径。

| Method | Public path | Purpose |
|---|---|---|
| GET | `/api/resources` | 列出当前登录用户的教学资源 |
| POST | `/api/resources` | 上传教学资源（PPT/PPTX、MP3、PDF、DOC/DOCX、TXT；单文件最大 25 MB） |
| GET | `/api/resources/trash` | 列出当前用户资源回收站 |
| DELETE | `/api/resources/:id` | 将本人资源移入回收站（软删除） |
| POST | `/api/resources/:id/restore` | 恢复回收站中的资源 |
| DELETE | `/api/resources/:id/permanent` | 永久删除回收站记录及服务器文件 |
| GET | `/api/resources/:id/download` | 下载本人未删除的资源文件，需登录 |
| GET | `/api/content` | 列出当前登录用户的内容，最多 500 条 |
| GET | `/api/content/:id` | 读取本人内容详情 |
| POST | `/api/content` | 创建草稿 |
| PATCH | `/api/content/:id` | 更新标题、别名、类型、摘要、正文、可见范围；修改前在同一事务保存历史快照 |
| DELETE | `/api/content/:id` | 软删除本人内容并移入回收站 |
| GET | `/api/content/:id/versions` | 查看本人内容的最近 100 个历史版本 |
| POST | `/api/content/:id/versions/:versionId/restore` | 恢复历史版本；先保存当前版本，已发布内容会写入验证事件 |
| GET | `/api/content/trash` | 列出本人回收站内容 |
| POST | `/api/content/:id/restore` | 从回收站恢复内容 |
| DELETE | `/api/content/:id/permanent` | 永久删除回收站中的内容及其历史版本 |
| POST | `/api/content/:id/publish` | 将公开内容发布，并在同一事务写入验证事件 |
| POST | `/api/content/:id/unpublish` | 下架本人内容 |
| GET | `/api/public/content` | 列出公开、已发布且未删除的内容 |
| GET | `/api/public/content/:slug` | 读取公开内容详情 |

管理端所有接口都要求有效登录会话，并按作者 ID 过滤。公开读取接口只返回 `status=published`、`visibility=public` 且未软删除的内容。内容默认保存为草稿且仅自己可见；发布前必须显式选择公开可见范围。

## Content rules

- Types: `article`, `tool`, `resource`, `project`.
- Statuses: `draft`, `published`, `unpublished`.
- Slug: lowercase ASCII letters/digits joined by single hyphens, max 100 characters, unique among non-deleted records.
- Title: 1–200 characters; summary max 500; body 1–100000 characters.
- Deletion sets `deleted_at`; the row is retained in the owner's recycle bin until restored or permanently deleted. Restoring a slug that has since been reused returns HTTP 409; the user must resolve the slug conflict first.
- Before a changed edit, the API locks the content row and stores the complete prior content snapshot in `portal.portal_content_versions` in the same transaction as the update. No-op saves do not create duplicate versions. Restoring a historical version first snapshots the current state, so the restore itself is reversible. Version history is scoped to the content owner; permanent deletion cascades to its snapshots.
- The editor uses a Notion-style WYSIWYG block workspace (formatting toolbar, headings, lists, task lists, quotes, code blocks, tables, images, links, undo/redo, and `/` block insertion). The editor serializes to Markdown in the existing `body` field, preserving compatibility with existing content records and public pages. Public Markdown is rendered with `marked` and sanitized with DOMPurify; raw HTML is escaped instead of trusted as author markup, and executable URL schemes and unsafe HTML attributes are removed.
- Publishing validates fields in the Node.js business layer and writes a `portal.content.validate` event in the same database transaction as the status update. The Go processor stores the validation result in `portal.event_inbox.processing_result`; event notification to the C IPC core is asynchronous and does not itself mean the public page was rendered.

## Deploy / verify

1. Apply unapplied migrations in order as a database administrator. On the current deployment, migration 007 is already applied and migration 008 adds version history and recycle-bin support.
2. Run `node --check backend/auth-api/server.mjs`, `node --check backend/content-api/handlers.mjs`, and `node --test backend/content-api/handlers.test.mjs`.
3. In `frontend/`, run `npm test`, `npm run type-check`, and `npm run build`. Markdown security tests live in `frontend/test/markdown.test.mjs`; editor Markdown round-trip compatibility tests live in `frontend/test/document-editor.test.mjs`.
4. Restart `personal_portal_auth_api` and atomically switch the frontend release symlink.
5. Verify unauthenticated management requests return 401, CRUD operations, public visibility rules, and that the publish validation event reaches `completed` with `valid=true`.

Do not run migrations automatically from the application startup path.


教学资源元数据保存在 `portal.teaching_resources`。资源文件存放于 Web 根目录之外的 `/var/db/personal-portal/resources`，下载接口校验登录用户所有权；移入回收站仅软删除元数据，永久删除才清理文件。迁移文件为 `backend/migrations/011_teaching_resources.sql`。
