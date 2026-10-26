# 独立文档编辑器架构

## 决策

- 前端：独立 Vue 3 + Vite SPA，生产路径 `/editor/`，输出目录 `frontend/dist-editor/`。
- API：复用既有 Go API，不引入第二套业务服务或数据库。
- 认证：同源 `/api/auth/me`，沿用 `portal_session` HttpOnly、Secure、SameSite=Strict Cookie。
- 内容接口：`GET /api/content/{id}`、`POST /api/content`、`PATCH /api/content/{id}`；图片仍走 `POST /api/uploads`。现有 Go API 已处理登录校验、作者归属、字段验证、历史版本和 PostgreSQL 持久化。
- 部署：Caddy 将 `/editor/` 映射到当前 release 的 `editor/` 子目录；编辑器 HTML 不缓存，带内容哈希的静态资源使用 immutable 缓存。SPA fallback 只回落至 `/editor/index.html`，不会回落至门户首页。
- 入口：门户新建与编辑操作转至独立编辑器；旧的 `/content-management/new` 与 UUID 编辑路径会重定向至新入口。
- 自动回退仍处于全局禁用状态。编辑器静态部署不需要重启 Go/C 服务。

## 数据与安全规则

1. 正文只以 Markdown 字符串提交，服务端仍是最终校验边界；不把编辑器 HTML 直接存为可信内容。
2. 编辑器在初始化时调用 `/api/auth/me`；401 时返回门户登录页。所有请求显式使用 same-origin credentials。
3. 新内容默认私有；保存只创建或更新草稿，不会自动发布。
4. 标题变化时自动生成 URL slug；用户手动编辑 slug 后停止自动覆盖。
5. 提交前校验标题、slug、正文和标签。标签按当前服务端允许的目录校验，自动补齐合法父分类；发现已不在目录中的历史标签时要求人工移除，不静默丢弃后再保存。
6. 存在未保存变更时显示状态，支持 Ctrl/Cmd+S，并在离开页面时提示。当前不把未保存正文写入 localStorage，避免将可能敏感的文档复制到持久化浏览器存储。
7. 图片上传沿用现有 Go API 的类型、大小和授权校验；链接和图片 URL 沿用现有编辑器 URL scheme 校验。

## 编辑器内核选择

本阶段保留现有 Tiptap 3 及自定义 Markdown 扩展，暂不迁移到其他编辑器。原因是已有内容包含自定义 callout、对齐块、文本颜色、背景高亮、表格和任务列表等语法；更换内核会扩大格式往返和历史内容兼容风险。

外部项目用于设计参考，不直接复制其实现：
- Tiptap Markdown：自定义 tokenizer / parse / render 扩展方式。官方文档明确 Markdown 扩展仍有 Beta 限制，包括注释支持和表格单元格多子节点限制，因此必须持续执行往返测试。
- Vditor：参考 WYSIWYG、即时渲染、分屏预览、目录、实时保存与移动端适配的功能分层；本阶段没有引入 Vditor 依赖。
- Milkdown：参考 core、plugin、component 分层以及可自定义主题的模块化设计；本阶段没有引入 Milkdown 依赖。
- TOAST UI Editor：参考 Markdown/WYSIWYG 模式、即时预览、滚动同步和代码高亮的交互方案；本阶段没有引入 TOAST UI Editor 依赖。

## 构建与验证

- `npm run build` 先构建并检查门户，再构建独立编辑器。
- `npm run type-check` 覆盖 `frontend/editor-app/` 与门户源代码。
- `npm test` 包含 Markdown round-trip、XSS / URL scheme、工具栏动作、标签 taxonomy 与独立编辑器入口合同测试。
- `npm run api:lint`、`npm run api:check` 检查 Go API 的 OpenAPI 契约。
- FreeBSD 部署脚本将 `frontend/dist-editor/` 复制至 release 的 `editor/` 目录。

## 已知限制与后续事项

- Tiptap / ProseMirror 引擎仍约 227.5 KiB gzip；独立入口移除了门户其他页面代码，但不能消除编辑引擎自身的体积。
- 编辑器 Markdown 引擎仍需以测试覆盖复杂表格、自定义语法、粘贴内容和旧文章往返转换；Tiptap 官方将 Markdown 扩展标注为 Beta。
- 当前保存为显式保存，不做自动服务器保存；避免每次编辑产生过多历史版本。
- 当前未在本阶段增加 Markdown 源码模式或分屏预览。若未来引入，先定义自定义语法与格式往返兼容标准，再决定依赖和交互方案。
- 上线后需在真实浏览器核验桌面与窄屏布局、登录态、图片上传、创建/编辑/版本历史以及刷新恢复。

## 官方参考资料

- Tiptap Markdown：<https://tiptap.dev/docs/editor/markdown>
- Vditor 仓库及 README：<https://github.com/Vanessa219/vditor>
- Milkdown Getting Started：<https://milkdown.dev/docs/guide/getting-started>
- Milkdown 架构概览：<https://milkdown.dev/docs/guide/architecture-overview>
- TOAST UI Editor：<https://ui.toast.com/tui-editor/>
- Mozilla MPL 2.0 FAQ（许可证处理规程相关）：<https://www.mozilla.org/en-US/MPL/2.0/FAQ/>
