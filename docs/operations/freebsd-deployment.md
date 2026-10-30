# FreeBSD 部署与服务管理

目标系统：FreeBSD 15.1-RELEASE-p6 / amd64。生产业务 API 只使用 Go。Node.js 后端源码已从 Git 项目移除；当前部署和回滚流程不再依赖或启用旧 Node.js API，正常生产流量不会经过 Node.js。

**环境区分：** 本文中的 `deploy.sh`、`Caddyfile`、`health-check.sh` 和 `rollback.sh` 是正式生产发布模板（API 8081、数据库 `personal_portal`）。当前公网个人门户实际运行的是开发版（API 8082、数据库 `personal_portal_dev`），必须使用 `deploy-dev.sh` 和 `Caddyfile.dev-public`；开发部署不会调用生产脚本。两套数据库、二进制目录、存储目录和 Caddy 配置不得混用。

## 生产拓扑

- Caddy 负责 HTTPS、静态前端、上传文件与反向代理；公共静态站点配置 `nosniff`、禁止 iframe 嵌入、严格跨源 Referrer Policy 与限制摄像头/麦克风/定位的 Permissions Policy。
- 网站由 Cloudflare 代理时，Caddy 只信任 Cloudflare 官方 IPv4/IPv6 CIDR，并只从 `CF-Connecting-IP` 解析客户端地址；Go 只接受 Caddy 通过 `X-Portal-Client-IP` 注入的已解析地址。Cloudflare 更新 IP 段时应同步更新 Caddyfile 并验证，不要信任任意来源的转发头。
- /api/* 去除 /api 前缀后代理至 127.0.0.1:8081 的唯一 Go API。
- 内容管理编辑器已集成到门户自身的 Vue 应用与 `/content-management/new`、`/content-management/:id` 路由，不再发布独立的 `/editor/` SPA。旧 `/editor/` URL 仅保留兼容跳转；构建和部署不会再复制独立编辑器应用。
- Caddy 明确拒绝公网访问 /api/events、/api/dbz、/api/readyz 和 /api/metrics。这些控制端点只供本机部署、健康检查与监控使用。
- Go 使用 database/sql + pgx，负责认证、会话、内容、历史版本、回收站、资源文件、公开内容互动、事件接收、分发、租约、重试和业务处理。
- C 事件核心仅使用 FreeBSD kqueue/kevent 和 Unix Socket 传送事件 ID/type 元数据；业务 payload、持久化、租约与处理结果只由 Go/PostgreSQL 处理。
- C IPC 协议为 v2。推送由 Go 使用 ACK 确认接收；PUBLISHED 和 ACKNOWLEDGED 都不是业务完成凭证。数据库状态是恢复和业务完成的唯一事实来源。
- Go 与 C 服务由 FreeBSD rc.d 管理，服务用户为 personalportal，socket 位于 /var/run/personal-portal/event.sock，权限为 0600。
- 日志位于 /var/log/personal-portal/；数据库连接 URL 存放于 /usr/local/etc/personal-portal/database-url，权限为 0640，不得提交到 Git 或打印到日志。

## 一键部署

部署脚本要求仓库干净且所有变更已经提交。脚本会执行许可证/资产预检、OpenAPI lint 与类型生成一致性检查、前端类型检查/测试/生产构建；随后创建独立临时 PostgreSQL 数据库，按顺序应用迁移 001–031 并运行 PostgreSQL 迁移/运行角色测试与 Go race tests，结束后自动删除测试数据库。之后运行 Go vet/build、C 事件核心编译与 IPC 测试，创建完整生产 PostgreSQL 备份并执行尚未应用的迁移，再安装 Go/C 二进制、切换门户静态 release 与 Caddy 配置，最后执行线上健康检查。若检测到旧 `personal_portal_task_executor` 服务、启用项或残留 root worker，必须显式设置 `PERSONAL_PORTAL_RETIRE_COMMAND_EXECUTOR=1` 才会调用退役脚本；脚本停止并禁用旧服务、确认无残留进程后删除专属运行目录，保留历史日志与数据库备份。

    cd /opt/portal-static-assets
    git status --short
    ./deploy/freebsd/deploy.sh

生产部署过程会在 `/var/backups/personal-portal/` 保存数据库 dump、旧前端 release 目标、旧 Go/C 二进制和旧 Caddyfile。部署成功后会停止并禁用已退役的 `personal_portal_auth_api` 服务；生产 Caddy 模板只将业务 API 转发到 8081，不转发到开发端口 8082。命令任务执行器在显式授权后退役，不再随部署安装或启动。完整回滚只恢复上一版 Go/C 二进制、Caddyfile 与前端 release，不会重新启用 Node.js API 或 root 命令执行器，也不会自动恢复数据库。

**数据库迁移安全：** 发布前先生成并保留完整 PostgreSQL dump。迁移按版本顺序执行；包含显式 `BEGIN/COMMIT` 的迁移由自身控制事务，历史上没有显式事务的迁移由 `psql --single-transaction` 包裹，避免单个迁移中途失败留下半套 DDL。迁移采用向前兼容优先的策略，不在部署失败时自动执行反向 SQL，也不会自动恢复整库 dump。迁移 031 是经负责人明确授权的破坏性退役迁移：删除命令队列/心跳/运行模式表和旧管理权限，并清除尚未分发的旧命令唤醒事件；必须先完成完整 dump 并确保旧 root worker 已停止。恢复数据库属于单独的人工恢复决策，必须先评估发布后新增数据是否会丢失。

**部署期保护规则：自动回退入口已全局禁用。** 无论 `PERSONAL_PORTAL_RELEASE_MODE`、`PERSONAL_PORTAL_AUTO_ROLLBACK` 如何设置，部署失败后都不会自动调用 `rollback.sh`；脚本只输出诊断目录与人工恢复提示，并保留 `/var/tmp/personal-portal-deploy-STAMP` 诊断材料。自动回退策略函数始终返回禁用，Shell 测试覆盖 development/stable/production 与多种环境变量组合。手动回退脚本保留，只有操作者明确执行时才会恢复旧 Go/C 二进制、Caddyfile 与前端 release。数据库备份与迁移前置检查仍然执行；手动回退不等于数据库反向迁移。

**旧编辑器 URL 兼容：** Caddy 将旧 `/editor/` 地址重定向到门户内置编辑器路由，不再提供独立编辑器静态文件，也不存在独立编辑器发布回退脚本。


## 当前开发版部署（公网测试环境）

当前公网门户由独立开发栈提供服务。开发 API 监听 `127.0.0.1:8082`，数据库为 `personal_portal_dev`，事件 socket 为 `/var/run/portal-static-assets-dev/event.sock`，上传和资源文件分别位于 `/var/db/portal-static-assets-dev/uploads` 与 `/var/db/portal-static-assets-dev/resources`。运行用户为 `personalportal`，注册默认关闭。Caddy 的 `Caddyfile.dev-public` 将 `/api/*` 去前缀转发到开发 API，并将 `/uploads/*` 原路径转发到 Go API 的图片权限检查；不得使用静态 `file_server` 直接暴露上传目录。

完成源码验证、GitHub 同步并提交后，开发部署命令为：

    cd /opt/portal-static-assets
    PORTAL_STATIC_ASSETS_DEV=1 ./deploy/freebsd/deploy-dev.sh

该脚本只允许 `main` 与 `origin/main` 完全一致、工作区干净时运行；先执行前端/Go/C 测试，再备份 `personal_portal_dev`、当前二进制、静态文件、uploads/resources 文件目录和 Caddy 配置，随后只迁移开发数据库、更新开发二进制/静态文件、启动开发 API/C 核心、reload Caddy 并运行 `health-check-dev.sh`。失败时保留备份和诊断材料，不自动回滚；不会操作生产数据库、生产二进制目录或正式生产 Caddy 模板。`dev-runtime.sh` 仅管理开发 API/C 核心，不自动启停 PostgreSQL 或 GSM。

开发版检查：

    ./deploy/freebsd/dev-runtime.sh status
    ./deploy/freebsd/health-check-dev.sh

开发部署不是正式发布。项目完成后，生产发布仍需单独进行架构/安全验收、备份和恢复演练及发布决策。

## 数据库迁移规则

- 迁移文件位于 backend/migrations/，编号顺序是唯一执行顺序；portal.schema_migrations 记录已应用版本。
- 生产部署先创建完整数据库备份，再应用未登记的迁移。迁移脚本使用受保护的数据库 URL 文件设置 libpq 环境变量，不将密码放入命令行参数。
- 迁移必须向前兼容当前应用；不要在部署失败时自动删除新表或倒退数据库结构。
- 当前恢复策略是先回滚应用代码和流量。若迁移包含破坏性或不兼容的数据变更，只有在评估备份时间点之后的数据损失并获得明确批准后，才恢复整个数据库 dump。
- 运行迁移脚本时必须显式提供备份文件：

    PERSONAL_PORTAL_MIGRATION_BACKUP=/var/backups/personal-portal/pre-go-cutover-STAMP.dump ./deploy/freebsd/migrate.sh

## 健康检查与监控

    ./deploy/freebsd/health-check.sh
    service personal_portal_api status
    service personal_portal_event_core status
    service caddy status
    tail -n 100 /var/log/personal-portal/api.log
    tail -n 100 /var/log/personal-portal/event-core.log

- /healthz：Go HTTP 进程存活状态。
- /dbz：PostgreSQL 连通性。
- /readyz：数据库与 C IPC 核心均可用。
- /metrics：数据库派生的 Prometheus 指标，包括各业务状态事件数、待分发事件数、过期处理租约数、最老待处理事件年龄。
- C 核心日志记录订阅连接、推送入队、ACK 成功/失败和有界队列溢出。
- PostgreSQL 的 portal.event_inbox、租约、重试时间和 processing_result 是事件恢复的权威依据。C 核心重启会丢失内存队列，但 Go 的周期扫描和过期租约回收会重新发现未完成事件。

## 回滚

开发期不会自动执行回滚。确认需要恢复时，使用部署输出的时间戳手动执行：

    ./deploy/freebsd/rollback.sh DEPLOYMENT-STAMP

回滚会恢复旧 Go/C 二进制、Caddyfile 和前端 release，并重新启动 Go API 与 C 事件核心。Node.js API 和已退役的命令任务执行器均保持禁用。回滚不会自动恢复数据库，因为这会丢失备份时间点之后的新数据；如确实需要数据库恢复，应先评估影响并单独执行。生产部署和回滚会改变正式在线服务，必须在取得单独授权后设置 `PERSONAL_PORTAL_ALLOW_SERVICE_RESTART=1` 执行。

## 生产验证覆盖

CI 和目标主机测试覆盖：

- Go 单元测试、竞态检测、静态分析与 PostgreSQL 集成测试。
- 全量 SQL 迁移从空数据库按顺序执行。
- OpenAPI 规范 lint、TypeScript 生成结果漂移检查和 Vue/TypeScript 类型检查。
- C IPC v2 的版本校验、请求超时边界、ACK 匹配、重复/错误 ACK、单订阅连接有界 FIFO、并发客户端和 SIGTERM 退出。
- FreeBSD 上真实 C 核心重启后 Go 订阅自动重连并重新收到推送。
- 发布后的本机 readiness、公开 API、认证边界、内部路由隔离和前端 HTTP 状态检查。
