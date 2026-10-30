# Personal Portal

## 当前架构决策（2026-10-30）：移除 Vue，保留后端职责边界

第一阶段 Vue 移除已完成并通过当前仓库验证：`frontend/src/` 不再包含 Vue SFC，Vue runtime、Vue Vite 插件及 Vue 类型检查依赖已移除。前端现为 **原生 TypeScript 模块 + 标准 DOM API + HTML/CSS**，以 `@preact/signals-core` 提供轻量响应式状态；Tiptap Core / ProseMirror 保留，Vue adapter 已移除。第一阶段 Vue 移除完成时的基线提交为 `c31036badc52f58f251dde7f04958bb3ee55caef`。第二阶段在此基础上继续审查全栈模块边界、业务契约、安全、数据库、C IPC 与验证体系；CSS 以证据驱动渐进整理，不做无关视觉重写。

**职责边界固定，具体实现允许重构。** Go 继续负责唯一业务 API 与后端业务规则；PostgreSQL 继续作为业务/事件持久化事实源；C 继续负责 FreeBSD 原生事件驱动核心及受限元数据通知，不接管业务 payload、业务规则或持久化；Caddy 继续负责 TLS、静态资源、路由、反向代理、可信代理与安全响应头。上述层的代码、模块、配置和内部实现均可在遵守职责边界、协议/数据兼容性、安全要求和验证流程的前提下重构。不得把“职责不可变”误解为“代码/配置冻结”；也不得因重构而把某层的职责转移给另一层。

前端迁移必须同步调整路由解析、模块挂载/卸载、状态所有权、事件监听器清理、测试与构建流程。每次变更先追踪调用链、数据流和依赖，再做最小迁移；测试、GitHub Actions 与真实浏览器 E2E 分开报告。不得把单元测试或 HTTP smoke 说成 E2E 通过。


FreeBSD 原生部署的个人知识与工具门户。项目由 GitHub 管理，不使用 Docker。

## 文档说明

本 README 同时作为项目入口、架构基线、实施记录与后续工程规则的唯一维护文档。架构、协议、数据库迁移、测试、部署和后续优先级统一在下文维护。

## 当前技术方向

- 前端目标架构：原生 TypeScript 模块 + 标准 DOM API + HTML/CSS；移除 Vue 运行时、SFC、组件模板编译与 Vue 状态/生命周期机制。Vite 可暂时保留为构建工具，是否保留及其插件链需在迁移中验证；生产环境只提供构建后的静态文件。
- 事件驱动核心：必须使用 C 语言实现，并优先利用 FreeBSD 原生 `kqueue` / `kevent`。
- 后端：唯一业务 API 使用 Go（`database/sql` + pgx），统一处理认证、会话、内容、教学资源和事件生命周期；C 仅负责 FreeBSD 本地 IPC/kqueue 元数据通知，PostgreSQL 保存业务与事件持久状态。
- 服务管理：FreeBSD `rc.d`。
- 内容：初期由项目自身管理，不接入 Notion。
- 教学资源：资源类型与适用年级分开保存；适用年级复用 `frontend/src/utils/taxonomy.ts` 的“年级”子项，未限定年级保存为 `NULL`；数据库通过版本化迁移维护字段，Go API 校验并返回年级，前端创建/编辑表单和表格共用该字段。
- 分类字段：文章内容独立保存父类与子类，年级与教育人力资源分类由共享 taxonomy 定义驱动。移除 Vue 后，自定义下拉菜单迁移为原生 TypeScript/DOM 实现；设计参数继续优先复用 `frontend/src/monochrome.css` 中的 `--portal-select-*` CSS 变量。
- 依赖原则：优先标准库和系统原生能力；没有明确需求时不增加第三方服务。

## 目录

- `frontend/`：原生 TypeScript 模块、DOM 操作、HTML/CSS 与前端构建/测试工具。
- `backend/api/`：Go HTTP API 与持久化事件接收入口。
- `event-core/`：C 语言 `kqueue` 事件核心；IPC v2 提供 PING/STATUS/PUBLISH/SUBSCRIBE/ACK 元数据通知。PUBLISHED 与 ACKNOWLEDGED 都不是业务处理完成确认。
- `deploy/freebsd/`：FreeBSD 部署与 `rc.d` 脚本。
- `docs/`：产品、架构、安全和运维文档。

## 本地构建前端

前端构建需要 Node.js 24 系列及 npm；生产业务 API 仅使用 Go 1.24+ 与 PostgreSQL。FreeBSD 当前通过 `go127` 提供 Go 工具链，部署脚本会自动检测 `go` 或 `go127`。

```sh
cd frontend
npm ci
npm run type-check
npm run build
```

`npm run build` 还会执行 `frontend/scripts/check-bundle-budget.mjs`：校验首屏 JS/CSS 与编辑器/Markdown 异步 chunk 的 gzip 预算，并防止编辑器依赖意外进入首页 HTML 的 preload 列表。生产构建产物位于 `frontend/dist/`，不应将其作为源代码提交。

## 当前状态

当前实现基线为原生 TypeScript/DOM 前端、单一 Go 业务 API、PostgreSQL 持久化与 FreeBSD 原生 C `kqueue` 事件核心。C IPC v2 的 `PUBLISH` / `ACK` 只确认通知扇出与接收，不代表业务完成；Go 是业务 payload 的唯一处理者，PostgreSQL 是恢复依据。2026-10-30 已将 Go `go test -race ./...`、C `make test` 和 PostgreSQL 空库迁移/运行角色权限检查加入 CI。2026-10-31 新增迁移 031，完整移除特权命令任务执行器及其数据库对象；FreeBSD 隔离 PostgreSQL 集群已实测 001–031 共 31 个迁移全部通过，并验证旧任务表、旧执行策略、旧管理权限及待处理命令唤醒事件均已清除；本地 Go race tests、`go vet`、前端测试/类型检查/构建和 OpenAPI 检查通过。FreeBSD 本机不支持 Playwright；Chromium E2E 以 GitHub Actions 结果为准。Go 测试现覆盖纯函数/HTTP 边界、并发事件幂等写入、处理租约归属/过期/重试、分发租约过期重领；仍未覆盖进程崩溃恢复、Go/C 真实 IPC 重连和端到端服务重启。C 当前测试验证协议解析与订阅 ACK/FIFO 状态，不覆盖真实 kqueue 并发、信号退出或服务重启。真实 FreeBSD 端到端验证仍需单独记录。部署脚本仅作为受控部署工具；第二阶段开发工作不代表生产部署授权。

---

## 架构基线、实施记录与后续工程规则

> 本文件是本项目的长期架构约束与运维执行基线。所有后续开发、修复、测试、提交和部署都必须遵循本文；如果实现、旧文档或临时方案与本文冲突，先核实事实，再更新文档和代码，不得悄悄引入第二套规则。
>
> 当前源码仓库：`lycupay-debug/portal-static-assets`；当前开发工作区：FreeBSD `/opt/portal-static-assets`。该仓库与既有生产部署目录分离；此处记录的开发提交、构建产物和验证结果不等于生产发布。生产拓扑和生产操作必须按独立审批流程核对。

## 1. 不可随意改变的架构原则

1. **Go 是唯一生产业务后端。** 认证、会话、权限、输入验证、业务 API、错误格式、数据库事务、内容和教学资源、事件接收/分发/处理/重试/恢复均由 Go 负责。禁止新增 Node.js/Python/C 业务 API，也禁止把同一规则分别实现于两个后端。
2. **Node.js/npm 仅用于前端开发与构建。** TypeScript 类型检查、测试、代码生成和打包可以使用 Node.js；前端运行时目标为原生 TypeScript 编译产物与标准 DOM API，不依赖 Vue。生产环境不依赖常驻 Node.js 业务服务。历史 Node 后端归档不属于正常运行架构。
3. **C 只承担底层事件通知与 IPC。** C 事件核心使用 FreeBSD `kqueue`/`kevent` 和 Unix Domain Socket，传递受限的事件元数据（如 `event_id`、`event_type`），不接收、解析、保存或执行业务 payload，不负责数据库、认证、幂等、租约、业务重试和处理结果。
4. **PostgreSQL 是业务和事件持久状态的事实来源。** C 的内存队列和推送只是唤醒提示；进程重启、丢失通知或重复通知后，Go 必须能从数据库状态恢复工作。
5. **OpenAPI 是 HTTP API 的共享契约源。** 先更新 `api/openapi.yaml`，再生成 TypeScript 类型并实现 Go 端行为；不得在 Vue 和 Go 中各自维护不一致的请求/响应结构。
6. **数据库结构通过版本化 SQL 迁移演进。** 不在应用启动时自动改 schema；迁移要有明确事务边界、约束和索引，并按编号顺序执行。
7. **FreeBSD 原生部署，不使用 Docker 作为生产部署方式。** 服务由 `rc.d` 管理，Caddy 负责 HTTPS、静态文件和 API 反向代理。
8. **开发期禁止自动回滚。** 当前 `PERSONAL_PORTAL_RELEASE_MODE=development`，`PERSONAL_PORTAL_AUTO_ROLLBACK=0`。只有项目明确进入稳定期并且操作者显式开启 stable + auto-rollback 时，部署脚本才可自动回滚。
9. **先证据、后结论。** 代码实现、测试通过、部署完成和生产验证是不同状态，必须分别记录。不得把本地测试通过描述成线上已经生效。
10. **每次变更必须形成可追踪提交。** 完成修改后执行相关测试、检查 diff、commit 并 push 到 GitHub；部署前要求仓库干净且提交已同步。除非遇到重大架构/数据风险，不需要逐步等待额外指令。

## 2. 目标架构与责任边界

```text
用户浏览器
   │ HTTPS
   ▼
HTML + CSS + 原生 TypeScript/DOM（Node.js 只参与开发/构建）
   │
   ▼
Caddy / HTTPS / 静态文件 / 安全响应头 / 反向代理
   │
   ▼
唯一 Go API（127.0.0.1:8081）
   ├── 认证、会话、权限、参数校验、统一错误响应
   ├── 内容、版本历史、回收站、公开互动、教学资源
   ├── PostgreSQL 事务、约束错误映射与业务数据访问
   ├── 事件接收、幂等、分发、租约、重试、业务处理和恢复
   └── 版本化 Unix Domain Socket IPC v2（只发送事件元数据）
             │
             ▼
       C 事件核心（FreeBSD kqueue / kevent）
       ├── 有界连接与元数据队列
       ├── 通知扇出、ACK 配对、超时和日志
       └── 不承载业务 payload 或持久化状态

PostgreSQL（portal schema）
   ├── 用户、会话和业务数据
   ├── 事件收件箱、分发状态、租约、尝试与结果
   ├── 幂等和状态约束
   └── schema_migrations
```

### 2.1 技术职责

| 层 | 唯一职责 | 禁止事项 |
|---|---|---|
| 原生 TypeScript + DOM | UI、交互、明确的模块状态与生命周期清理、调用类型化 API | 自行定义与 OpenAPI 冲突的 DTO；把客户端校验当成安全边界；引入 Vue 运行时依赖 |
| Node.js/npm | 前端依赖管理、代码生成、测试和构建 | 生产业务 API、认证服务、业务任务处理器 |
| Caddy | TLS、静态资源、路由、代理、可信代理配置、安全头 | 绕过 Go 直接连接业务数据库；向 Go 传入未经可信代理校验的客户端 IP |
| Go API | 全部 HTTP 业务语义、身份与授权、验证、错误格式、事务、事件生命周期 | 将业务规则下放给 C 或另建 Node 服务 |
| C event core | `kqueue`/`kevent`、Unix socket、事件元数据通知 | 业务 payload、数据库写入、持久化队列、业务完成确认 |
| PostgreSQL | 业务和事件持久化、约束、租约与恢复依据 | 依赖 C 内存队列来保证业务任务不丢失 |
| Python（辅助工具） | 在确有需要时执行运维自动化、数学计算、数据处理/分析、一次性诊断脚本 | 作为第二个常驻业务后端或绕过 Go 的业务规则层 |
| C / cgo | 当前独立 C 核心负责 FreeBSD 事件机制；未来只有性能分析证明必要时，才考虑通过 cgo 调用 C 动态库 | 未经基准测试就引入 cgo；用 cgo 替代当前清晰的 IPC 边界而不做架构评估 |

## 3. API 契约与统一业务规则（优先级 P1）

- 契约文件：`api/openapi.yaml`（OpenAPI 3.1）。
- 前端生成器配置：`frontend/openapi-ts.config.mjs`；生成代码位于 `frontend/src/api/generated/`。
- 规范变更流程：修改 OpenAPI → `npm run api:lint` → `npm run api:generate` → 检查生成 diff → 实现/调整 Go API → 更新契约与集成测试 → 全套 CI。
- CI 使用 `npm run api:check` 验证生成代码与契约同步。生成文件不是手工编辑的源头；要改字段必须修改 schema 后重新生成。
- Go 必须对请求体大小、JSON 格式、必填字段、未知字段策略、枚举、字符串长度、URL/路径、权限和资源归属进行服务端验证。前端校验只能改善体验，不能替代服务端验证。
- RBAC 角色/权限变更与 compliance reviewer 变更统一使用有界严格 JSON decoder；拒绝未知字段、尾随 JSON 值和超限请求体。合规 review_type 必须与迁移 022 的数据库 CHECK 一致：历史 copyright 类型归并为 license，不再接受旧枚举。
- 登录/注册速率限制的客户端路径键集合有 10,000 项硬上限；达到上限时先回收过期项，仍满则对新键 fail-closed 返回 429，避免攻击流量使进程内 map 无界增长。
- 认证、权限、参数校验、错误响应、事务和字段语义只能在 Go 中形成一套生产规则。数据库约束作为最后一道完整性保护，Go 负责把数据库错误映射为稳定、可预测的 API 错误。
- 错误响应结构必须统一并在 OpenAPI 中定义；不得新增仅某个路由使用的临时错误格式。HTTP 状态码、业务错误码和敏感信息披露边界需要通过测试固定。
- 任何字段重命名、必填性变化、枚举收窄、错误格式变化都视为契约变更；必须同时评估已有数据、前端兼容性和迁移顺序。

## 4. Go + C IPC v2 协议基线（优先级 P3）

协议详细说明见 [`docs/architecture/event-ipc-v2.md`](docs/architecture/event-ipc-v2.md)。本节列出不可违背的关键语义。

### 4.1 传输与帧

- Unix Domain Socket：`/var/run/personal-portal/event.sock`，权限 `0600`；不开放 TCP 监听。
- AF_UNIX / SOCK_STREAM，ASCII、LF 分帧；单行请求限制为 254 字节（LF 前）。
- 请求：`v2 <request_id> <command> [event_id event_type]`；响应必须回显相同 `request_id`。
- IPC v1 客户端拒绝。任何不兼容协议变化必须升级版本，并同步修改 C、Go 客户端、文档和测试。

### 4.2 确认语义

| 响应/命令 | 仅能证明 | 不能证明 |
|---|---|---|
| `PING` / `PONG` | 进程与 socket 可响应 | 数据库可用、事件已处理 |
| `STATUS` / `RUNNING` | C 事件循环正在接收 IPC | 业务处理器健康或事件已完成 |
| `EVENT` / `ACCEPTED` | 元数据通过检查 | 持久化成功或业务任务已排队 |
| `PUBLISH` / `PUBLISHED` | C 接受了一次元数据通知扇出尝试 | 所有订阅者已收到，或业务处理完成 |
| `ACK` / `ACKNOWLEDGED` | Go 确认收到了匹配的通知 | 业务处理完成、事务提交或事件进入终态 |
| PostgreSQL 处理结果 + 终态状态已提交 | 业务处理已按事务语义结束 | 不能被通知 ACK 取代 |

- Go 必须先把业务 payload 和事件元数据写入 PostgreSQL 并提交，之后才对事件接收方返回成功；再尝试发布通知。
- `event_id` 是 UUID 幂等标识。同 ID、同内容应作为重复请求安全处理；同 ID、不同内容必须冲突，不得覆盖原事件。
- C 的推送 FIFO 每个订阅者最多 32 条，单个订阅者同时最多有一条未确认推送。队列是内存结构，允许在断连/重启时丢失；溢出必须有日志，不能丢失数据库中的事实记录。
- 当前 Go 命令超时默认 750 ms；C 非订阅空闲客户端与未 ACK 的推送连接在 5 秒超时后断开。Go 订阅端在 EOF、超时、协议错误或 ACK 错误后重连，指数退避上限 10 秒。
- 重复通知是允许的。Go 必须依靠 PostgreSQL 状态检查、幂等键、处理租约和事务完成，避免重复执行业务效果。
- C 核心重启会丢失内存 FIFO 和现存连接；Go 必须重新订阅。未完成事件由数据库扫描、过期租约回收和重试重新发现，不能依赖 C 重放。
- 处理器输入错误等可预期业务拒绝必须产生稳定结果，不应无限重试；只有真实处理故障才进入重试策略。终态事件不得继续被领取。

### 4.3 事件可观测性要求

- 保留结构化日志：连接、订阅、推送入队、ACK 成功/失败、超时、队列溢出、重连、分发尝试、租约领取/过期、业务处理失败和最终状态。
- 监控应能观察待分发数量、待处理数量、最老事件年龄、过期租约、重试次数、处理耗时、分发失败和核心重连；内部指标端点不能直接暴露公网。
- 事件诊断必须区分通知层失败、数据库分发失败、业务处理失败和业务校验结果为无效。`completed` 代表处理流程已完成，不等价于业务校验 `valid=true`。
- 对历史出现过的异常高分发尝试次数，应通过日志、SQL 状态转移和可重复测试确定原因；不得仅因当前无积压就认定根因已解决。

## 5. PostgreSQL 与 SQL 迁移规范（优先级 P2）

- 数据库：`personal_portal`；业务 schema：`portal`；应用角色：`personal_portal_app`。不得访问 n8n 或 Host MCP 的数据库/凭据。
- 迁移目录：`backend/migrations/`，编号文件是唯一迁移顺序；`portal.schema_migrations` 记录应用版本。当前仓库包含 `001` 至 `031` 共 31 个迁移文件。新增迁移从 `032` 开始；不得改写已经应用到任何共享环境的历史迁移。
- 应用启动不自动执行 DDL。生产迁移通过 `deploy/freebsd/migrate.sh` 显式运行，先做完整 PostgreSQL dump，再按版本顺序应用。
- 每个迁移必须具备明确的事务语义：自带 `BEGIN/COMMIT` 的文件自行管理；不带显式事务的迁移由 `psql --single-transaction` 包裹。迁移失败必须停止，不得继续应用后续版本。
- 迁移必须检查主键、外键、唯一约束、`NOT NULL`、检查约束、必要索引和并发访问成本。索引要服务于实际查询与租约领取模式，不盲目增加。
- 采用向前兼容优先的扩展/迁移/收缩流程：先添加兼容字段/索引，再发布兼容代码和回填，确认无旧代码依赖后才考虑删除旧结构。
- 对大表或锁风险 DDL，先在接近生产规模的数据上评估执行时间、锁和回滚方式；必要时拆成多个迁移。
- CI 必须从空 PostgreSQL 测试数据库按序执行全部迁移，并运行 Go/PostgreSQL 集成测试。新增迁移还应覆盖约束失败、重复数据、并发和事务回滚等相关行为。
- 部署失败不自动执行反向 SQL，也不自动恢复数据库 dump。恢复整库可能丢失备份之后的数据，必须作为单独的恢复决策并评估数据损失。

## 6. 测试与 GitHub CI（优先级 P4）

统一入口为 `.github/workflows/frontend-ci.yml`，push、pull request 和手动触发均执行检查。每次提交至少覆盖：

1. **OpenAPI/TypeScript：** OpenAPI lint、生成文件同步检查、TypeScript 类型检查。
2. **前端：** 自动化测试、生产构建与 bundle 预算；首屏不得提前加载编辑器或文章 Markdown 渲染器。安全的 Markdown/URL 处理、编辑器块往返持久化、分类数据约束等逻辑须有测试。
3. **Go：** `go test -race ./...`、PostgreSQL 集成测试、`go vet ./...`、编译。
4. **SQL：** 空数据库全量迁移按序执行，并使用应用数据库角色测试权限与约束。
5. **C：** 编译启用 `-Wall -Wextra -Wpedantic -Werror`；测试协议版本、畸形/超长请求、ACK 匹配、重复/错误 ACK、FIFO、有界队列、并发连接、空闲/ACK 超时和 SIGTERM。
6. **部署脚本：** Shell 语法检查、迁移事务策略测试、部署策略测试、Caddy 安全配置检查和隔离沙箱回滚演练。
7. **生产验收：** 部署后检查 Go、C、PostgreSQL、Caddy 状态；健康/ready；公开页面和 API；认证边界；内部路由不可从公网访问；安全响应头和前端静态资源。

CI 的模拟回滚不等于真实生产恢复；部署后的健康检查也不等于每个业务功能都完成端到端验收。任何新功能都应补充对应 Go 集成测试、前端测试或 FreeBSD 原生验证，并在报告中标注实际运行环境。

## 7. FreeBSD 构建、发布与回滚（优先级 P4）

### 7.1 生产拓扑与安全

- FreeBSD 原生服务：`personal_portal_api`、`personal_portal_event_core`、`caddy`、`postgresql`；服务由 `rc.d` 管理。
- Go API 监听本机 `127.0.0.1:8081`。Caddy 对外提供 HTTPS 和静态站点，`/api/*` 去掉 `/api` 前缀后代理到唯一 Go API。
- Caddy 只信任 Cloudflare 官方 IPv4/IPv6 CIDR，并只从 `CF-Connecting-IP` 解析真实客户端 IP；向 Go 传递由 Caddy 解析的 `X-Portal-Client-IP`。禁止信任任意客户端自带的 `X-Forwarded-For` 或类似头。
- 内部端点 `/api/events`、`/api/dbz`、`/api/readyz`、`/api/metrics` 不得暴露公网；内部指标和诊断信息不能泄露凭据或个人数据。
- 数据库连接 URL 位于 `/usr/local/etc/personal-portal/database-url`，权限应受限；任何凭据、令牌和生产数据都不得提交到 Git 或打印到日志。
- 备份与诊断目录权限应最小化。Cloudflare IP 段发生变化时同步更新配置并验证，不能随意扩大可信代理范围。

### 7.2 发布标准流程

在 `/opt/personal-portal` 运行：

```sh
git status --short --branch
# 必须干净，所有变更已经 commit + push
./deploy/freebsd/deploy.sh
```

部署脚本负责运行前端生成/测试/构建、Go race tests/vet/build、C 编译与 IPC 测试、数据库备份、迁移、安装二进制、启动服务、等待 readiness、验证 Caddy 配置、切换前端 release、重载 Caddy 和线上健康检查。发布前后的版本路径、旧二进制、Caddyfile 和数据库 dump 应保留为恢复依据。

### 7.3 开发期失败策略

- 默认 development，`PERSONAL_PORTAL_AUTO_ROLLBACK=0`；失败时保留诊断材料并给出手动回滚命令，不自动切换版本。
- 只有明确进入稳定期，并同时显式设置 `PERSONAL_PORTAL_RELEASE_MODE=stable` 与 `PERSONAL_PORTAL_AUTO_ROLLBACK=1`，才允许部署脚本执行自动回滚。
- 回滚应用版本不等于回滚数据库。不要未经评估恢复旧 dump；数据库恢复需要独立评估备份时间点之后新增数据的损失。
- 生产配置切换前，必须确认配置语法有效、备份已完成、测试通过、服务依赖清晰，并有明确的发布后验证步骤。
- 部署失败后先收集服务状态、API/Caddy 日志、迁移记录、当前 symlink 和备份信息；除非进入稳定期并明确开启开关，不得自动清理诊断目录。

### 7.4 发布后验收清单

- [ ] `service personal_portal_api status` 正常。
- [ ] `service personal_portal_event_core status` 正常。
- [ ] `service caddy status` 正常。
- [ ] `sh deploy/freebsd/health-check.sh` 通过。
- [ ] `/healthz`、`/dbz`、`/readyz`、`/metrics` 本机检查通过。
- [ ] 首页、公开内容、认证边界返回预期状态码。
- [ ] `/api/events`、`/api/dbz`、`/api/readyz`、`/api/metrics` 公网不可访问。
- [ ] 安全响应头与当前仓库 Caddyfile 一致。
- [ ] `current` 指向预期 release；旧版本和数据库 dump 均可定位。
- [ ] 检查事件积压、最老事件年龄、过期租约和近期错误日志。
- [ ] GitHub CI 对发布提交的结果为成功。

## 8. Python 与 C/cgo 的使用边界（优先级 P5）

- Python 是辅助工具：用于运维脚本、批量数据处理、数学计算、数据分析、离线诊断或生成报告。脚本应可重复执行、输入输出明确、对生产数据默认只读，并记录执行结果。
- Python 不得成为第二个常驻业务 API，不得绕过 Go 直接实现认证、权限、内容或事件业务写入。需要写业务数据时，应调用 Go API 或经过评审的受控运维流程。
- 当前 C 事件核心是独立 FreeBSD 原生服务，Go 通过 Unix socket 与其通信。这个边界是有意设计的，不应仅因“可以 cgo”就把 C 核心改成动态库调用。
- 只有在 profiling/基准测试证明确有 CPU 密集或底层性能瓶颈，且 Go 标准库/系统调用无法合理解决时，才评估 cgo 调用 C 动态库。评估必须包含：性能收益、调用/内存所有权、线程与阻塞、错误传递、FreeBSD ABI/构建、race/sanitizer 覆盖、部署复杂度和维护成本。
- cgo 不应用来承载业务规则、数据库事务或事件持久化；业务状态仍由 Go + PostgreSQL 管理。

## 9. 已完成的实施与验证记录

以下表格保留既有 `personal-portal` 项目的历史实施与生产运维记录，主要用于追溯原架构决策。表中仓库、生产主机、部署日期和旧 CI 结果不代表当前 `portal-static-assets` 仓库的实时状态；第二阶段的当前基线与验证结果以本页顶部更新及 Notion《检查源码重构（第二阶段）》为准。

| 项目 | 当前状态 | 证据/范围 |
|---|---|---|
| Go-only 生产业务后端 | 已实施 | Go API 处理业务；旧 Node API 不在正常生产流量路径，部署后已禁用旧认证服务 |
| 原生 TypeScript + DOM（第一阶段） | 已完成 | 第一阶段完成提交 `c31036badc52f58f251dde7f04958bb3ee55caef`；Vue SFC 为 0，Vue runtime / 插件 / vue-tsc 依赖已移除；当前前端行为测试 106 项通过，类型检查与构建通过（2026-10-30 FreeBSD 开发工作区实测） |
| OpenAPI 共享契约 | 已扩展 | `api/openapi.yaml` 覆盖现有客户端路由、admin/RBAC、compliance、public legal 和根路径上传图片读取；角色/权限及合规写入请求使用明确 schema；生成 TypeScript 类型并纳入 lint/CI 同步检查 |
| PostgreSQL 版本化迁移 | 当前 001–031 全量空库回放通过 | `scripts/test-postgres-migrations.sh` 纳入 CI；FreeBSD 独立临时 PostgreSQL 集群实测 31 个迁移全部通过，验证 owner/runtime 分离、默认 user 角色触发器、禁止 runtime CREATE/TRUNCATE，以及命令执行器退役后的表/权限/事件清理；不触碰现有 PostgreSQL 服务或业务库 |
| Go 事件接收、分发、租约、处理和恢复 | 单元与部分 PostgreSQL 集成测试已纳入 CI | 覆盖并发幂等接收、处理租约唯一领取/归属校验/重试/最终尝试过期、分发租约过期重领；真实进程崩溃恢复、Go/C IPC 重连与服务重启仍未覆盖 |
| C IPC v2 | 协议实现存在；局部单元测试已补充 | 当前 `make test` 覆盖请求/事件标识校验、PING/STATUS/EVENT、v1 拒绝、未知命令、订阅推送、错配/成功 ACK、FIFO 顺序和 32 项上限；真实 socket/kqueue、并发客户端、超时、信号退出与重启恢复未由该测试覆盖 |
| C 核心资源边界 | 已实施 | C 只处理事件元数据通知，不处理业务 payload |
| 部署与恢复策略 | 已实施 | 备份、迁移、健康检查、手动回滚；开发期默认不自动回滚；隔离沙箱回滚演练 |
| 特权命令任务执行器 | 已移除（2026-10-31） | Go root worker、developer/stable 模式、任务队列/API/UI/rc.d 服务已从当前源码移除；迁移 031 删除相关数据库对象与管理权限；系统运维仅保留只读告警/日志及独立 RBAC 权限管理 |

门户不再提供远程命令执行、root 任务队列或执行策略切换功能。服务器维护继续通过独立授权的 GSM/SSH 运维通道完成；门户中的系统运维页面仅提供告警、日志和账户 RBAC 管理。
| 安全响应头与 Cloudflare 代理信任 | 已部署并验证 | 生产 Caddyfile 与仓库配置一致；本机及公网检查响应头；内部路由边界测试通过 |
| 历史架构/部署 CI | 历史记录 | `f7e8aba`：`https://github.com/lycupay-debug/personal-portal/actions/runs/38876797502`；该 run 属于旧 `personal-portal` 仓库，不作为当前 `portal-static-assets` 的 CI 证据 |
| 最新生产部署 | 已完成 | 2026-10-16 UTC；部署脚本输出 stamp `20261016T085236Z`；发布后 health-check 通过 |
| 数据库备份 | 已生成且可读取 | `/var/backups/personal-portal/pre-go-cutover-20261016T085236Z.dump`；`pg_restore -l` 可读取目录清单 |

### 已知待持续改进项

1. **编辑器前端 bundle 偏大。** 2026-10-16 FreeBSD 构建基线：编辑器 chunk 683.54 kB 原始 / 210.46 kB gzip；编辑器已通过异步组件按需加载。此次将公开文章 Markdown 渲染器改为文章路由动态导入，首页 JS 从 245.36 kB 原始 / 83.71 kB gzip 降至 168.92 kB 原始 / 58.56 kB gzip（gzip 后减少约 30%）；Markdown 渲染器独立为 76.26 kB 原始 / 25.29 kB gzip，仅在打开文章时加载。Rolldown 手动分组实验会让编辑器依赖进入首页预加载，因此未保留该配置。编辑器 chunk 的 500 kB 提示基于未压缩体积；后续优化必须验证首页网络请求没有提前加载编辑器，并以性能分析为依据，不为消除警告而盲目重构。
2. **事件分发历史高重试次数需要根因分析。** 曾观察到一个最终已完成的事件累计 41 次分发尝试；当前检查没有待分发积压，但“目前无积压”不能证明历史原因已消除。应通过状态迁移、重试日志、SQL 查询与可重复测试定位，不能直接修改生产事件记录来掩盖问题。
3. **Caddy/代理信任变化必须持续回归。** Cloudflare CIDR 可能更新；修改可信代理配置前应校验来源与格式，修改后同时做本机、经过 Cloudflare 的公网请求和安全头/限流行为测试。
4. **生产验证要与 CI 分开报告。** GitHub CI 在 Linux/macOS runner 上完成的测试，不自动等价于 FreeBSD 上所有故障场景均已验证；涉及 `kqueue`、`rc.d`、Unix socket 和服务重启的行为，应继续保留 FreeBSD 原生测试与部署后验收记录。

## 10. 后续实施优先级

### P1 — API 契约和规则一致性

- 每个新增/修改 API 先更新 OpenAPI，再生成 TypeScript 类型并完成 Go 实现。
- 补齐统一错误响应、请求校验、权限与资源归属测试；防止字段和错误格式漂移。
- 对认证、内容、互动、教学资源等主要端点维护 OpenAPI 与 Go 集成测试的对应关系。

### P2 — SQL、事务与数据完整性

- 评估关键表约束、索引、唯一性、外键、租约查询计划和事务边界。
- 为新迁移补齐升级失败原子性、重复数据、并发和兼容性测试。
- 对事件状态转移增加可验证约束，分析高重试次数及终态事件被重复领取的可能性。

### P3 — Go + C 协议与故障恢复

- 持续测试丢 ACK、重复 ACK、错配 ACK、慢订阅者、FIFO 溢出、C 核心退出/重启、Go 订阅重连、数据库短暂不可用和过期租约恢复。
- 明确区分“通知已送达”和“业务已提交”；任何代码/日志/指标都不能混淆这两个概念。
- 对重复/丢失通知保证幂等和最终恢复，并持续改善队列、重试、延迟和失败原因的可观测性。

### P4 — CI、部署与运维自动化

- 保持 GitHub CI 单一入口；新增目录/工具必须纳入同一套质量门禁，不再形成孤立脚本而无人运行。
- 部署前自动检查干净工作区、备份、迁移状态、Caddy 配置和构建产物；部署后检查实际服务和公网路由。
- 保持 FreeBSD 原生测试与沙箱回滚演练；开发期失败时先诊断，不自动回滚。

### P5 — Python 辅助和性能专项

- 需要批量数据、数学、分析或一次性运维自动化时，优先评估 Python 是否能减少复杂度，并保持它是辅助工具。
- 只有性能证据充分时才引入 cgo/C 动态库；先做基准测试与风险评估，再形成 ADR、实现和回归测试。

## 11. 每次变更的执行流程

1. 读取当前代码、相关文档、Git 状态和生产约束；先确认事实，不依据旧印象猜测。
2. 识别变更影响的契约、数据库、事件协议、安全边界和部署流程。
3. 先更新结构/契约与测试，再实现最小必要改动；避免同时改动无关模块。
4. 执行受影响模块测试，再执行全量 CI 对应检查；检查 `git diff --check`、生成文件漂移和秘密信息。
5. 更新本文或相应 ADR/运维文档，区分已实现、已测试、已部署、待验证。
6. commit 并 push；检查 GitHub Actions 对最新提交的结论。
7. 只有在测试、备份、发布策略和验收条件具备后才部署生产；部署后执行健康检查与业务边界验证。
8. 报告中明确列出提交、测试、CI、部署状态、备份位置、已知风险和下一项自主推进任务。

## 12. 需要重大判断时暂停的情况

仅在以下情况需要向项目负责人提出重大判断，而不是擅自改变方向：

- 需要删除或重写生产数据、恢复整库备份或执行不可逆迁移；
- 需要改变 Go-only 后端、C 事件核心责任边界、OpenAPI 契约治理或 PostgreSQL 持久化架构；
- 需要开放新的公网服务/端口、扩大可信代理范围、改变认证安全策略或暴露内部指标；
- 需要在稳定期启用自动回滚、切换破坏性发布策略或引入新的常驻服务/数据库；
- 性能测试表明需要引入 cgo、额外语言运行时或新基础设施，且影响维护/部署边界。

除此之外，按本文架构自行实施、测试、提交、推送和验证，不需要逐步请求确认。

