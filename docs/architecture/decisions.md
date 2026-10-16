# 架构决策记录

## ADR-0001：运行与部署方式
- 状态：已决定
- 项目直接部署在 FreeBSD 主机，不使用 Docker。
- GitHub 作为代码、版本和自动化检查的中心。
- 生产环境优先运行静态前端文件与独立后端服务。

## ADR-0002：前端技术栈
- 状态：已验证基础构建
- Vue 3 + TypeScript + Vite。
- Node.js/npm 用于开发和构建；生产环境不要求常驻 Node.js。
- 使用 `package-lock.json` 固定依赖解析结果；CI 使用 `npm ci`。
- CI 必须执行 Vue/TypeScript 类型检查和生产构建。
- 避免非必要的 UI 框架、状态管理库和开发插件。

## ADR-0003：C 事件驱动核心
- 状态：已实现并通过 IPC v2 自动化测试
- 事件驱动核心必须由 C 语言开发。
- 优先使用 FreeBSD `kqueue` / `kevent` 和标准 C 库。
- 事件通知与业务任务可靠性分离：内核通知本身不提供业务任务持久化、幂等、重试或恢复保证。
- IPC v2 已定义协议版本、请求 ID、超时、确认语义、有界 FIFO、重复/错误 ACK 行为和重连策略；数据库驱动的 Go 周期扫描负责丢失通知后的恢复。

## ADR-0004：后端语言
- 状态：已决定并已在 FreeBSD 生产目标验证
- Go 是唯一业务后端，使用 `net/http`、`database/sql`、pgx 和 Go bcrypt。认证、会话、内容、教学资源、错误格式、事务和事件生命周期均由 Go 负责。
- Node.js 仅用于 Vue/Vite 前端开发与构建，不再承担生产业务 API；历史后端代码从仓库移除，紧急回滚副本存放在 `/opt/personal-portal-legacy-node`。
- Go 与 C 通过版本化 Unix Domain Socket IPC v2 解耦；业务 payload 不进入 C 核心。

## ADR-0005：内容与存储
- 状态：已实现 PostgreSQL 持久化
- 暂不接入 Notion。
- 业务内容、历史版本、教学资源、互动与事件状态保存在 PostgreSQL；内容正文采用 Markdown 持久化。
- 公开内容和私人数据必须在查询、API 权限和数据字段层面隔离。

## ADR-0006：PostgreSQL 持久化基础
- 状态：已在 FreeBSD 主机完成业务表、版本化迁移和集成测试。
- 复用主机已有 PostgreSQL 17，不新建数据库服务。
- 门户使用独立数据库 `personal_portal` 与登录角色 `personal_portal_app`；不得访问 `n8n_db` 或 `host_mcp_oauth`。
- Go API 使用 `database/sql` 与 pgx stdlib 驱动；连接 URL 从主机受限权限文件读取，不提交到 Git。
- SQL schema 变更通过 `backend/migrations/` 中的版本化 SQL 管理；应用启动时不自动修改 schema。
- `portal.schema_migrations` 记录已应用的迁移版本；当前 schema 包含事件收件箱、处理租约/结果、账户与会话、内容、历史版本、互动、教学资源和父子分类字段。
- C `kqueue` 核心仍负责 IPC/事件通知；持久化事务与后续业务数据由 PostgreSQL 管理。


## ADR-0007：持久化事件接收
- 状态：事件接收、分发、业务处理、租约恢复与基础故障注入测试已实现；生产业务处理器仍仅覆盖 `portal.content.validate`，新增业务类型需单独定义契约并验证。
- Go API 先验证并将业务事件写入 `portal.event_inbox`，成功提交后才返回接收成功。
- 调用方提供 UUID `event_id` 作为幂等键；同 ID 同内容为重放，同 ID 不同内容返回冲突。
- C IPC v2 只传递事件 ID/type 元数据，并为订阅推送提供 ACK 与容量为 32 的有界 FIFO；不承载业务 payload。事件持久化、租约、重试和处理结果由 PostgreSQL + Go 管理。
- 不在 API 启动时自动执行数据库迁移。


## 当前事件处理契约：`portal.content.validate`

- 事件负责编校验内容草稿，不负责发布、写入 Markdown/JSON 文件或建立内容索引。
- 输入字段：`title`、`slug`、`content_type`、`tags`、`visibility`、`summary`、`body`。内容类型限定为 `article`、`tool`、`resource`、`project`；标签须来自门户允许的知识广场标签且不超过 40 个；可见性限定为 `public`、`private`。
- 校验结果保存在 `portal.event_inbox.processing_result`，与 `status='completed'` 在同一条 SQL 更新中写入。`completed` 表示校验处理已执行完，不代表内容通过校验；以结果中的 `valid` 字段为准。
- 可预期的输入错误（必填字段、格式、长度、未知字段或字段类型不符）返回 `valid=false` 和错误列表，不进入重试。仅处理器实际错误进入事件重试路径。
- 私人/公开可见性仅作字段合法性校验；此事件不会发布内容、生成公开产物或改变任何公开索引。
- 迁移 `005_event_processing_result.sql` 为事件收件箱添加结果字段。
