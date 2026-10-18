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
- `portal.schema_migrations` 记录已应用的迁移版本；当前 schema 包含事件收件箱、处理租约/结果、账户与会话、内容、历史版本、互动、教学资源、父子分类字段及系统运行模式策略。
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

## ADR-0008：系统运行模式与任务权限策略
- 状态：策略配置页面、持久化模式和分类判定规则已实现；通用命令执行器尚未接入。
- 管理入口为 `/account/system-ops/policy`，与系统警告、系统日志页面分离。
- `portal.system_execution_policy` 保存唯一当前模式（`developer` 或 `stable`）；模式变更在同一事务中写入 `portal.compliance_audit_log`。
- 两种模式均要求重启操作与 SSH_MCP 相关操作事先取得用户同意；稳定模式下部署、数据库变更和破坏性任务逐次审批。未知任务类别默认拒绝。
- 任何后续任务执行器必须在真正执行前调用后端统一策略判定器，并实施路径/工作区范围、审批、超时、进程组清理、幂等、审计与崩溃恢复；页面上的模式值不能被视为对尚未接入执行器的自动控制。


## ADR-0009：复用事件系统的受控命令任务执行器
- 状态：已部署；FreeBSD API、C 事件核心和独立 root 执行器健康检查通过；专用测试数据库验证了稳定模式降权、开发者模式 root 执行、重启审批、SSH_MCP 人工处理和高风险命令重分类。
- PostgreSQL `portal.command_tasks` 是任务生命周期的事实来源，幂等键唯一；`portal.event_inbox` 中的内部 `portal.command.execute` 事件只负责通过既有 C 事件核心唤醒，不承载可执行命令。
- 公开 `/events` 接口拒绝 `portal.command.execute`，避免外部请求直接触发特权执行。管理员会话 API 才能创建、审批、取消任务。
- 独立 root 执行器复用现有 API 二进制的 `--task-executor` 入口，独立 rc.d 服务运行；当前策略在执行前重新读取数据库，稳定模式低风险任务降权至 `personalportal`，经审批的高风险命令按每类受控清单执行。
- 命令通过绝对可执行路径与参数数组执行，不允许 Shell 拼接；进程组、超时、输出大小、取消请求、租约和审计均有边界。过期租约任务不自动重放，避免重复非幂等副作用。
- `restart` 分类始终要求审批；命令特征会触发强制重分类。`ssh_mcp` 以及识别到 SSH_MCP/host-mcp 的命令只标记为人工处理，绝不由执行器代执行。
- 稳定模式对低风险任务执行白名单校验；高风险审批后仍须匹配对应执行清单。开发者模式允许 root 运行用户授权范围内的 argv 命令，但不允许 Shell 字符串拼接。
