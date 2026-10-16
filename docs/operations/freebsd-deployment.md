# FreeBSD 部署与服务管理

目标系统：FreeBSD 15.1-RELEASE-p4 / amd64。生产业务 API 只使用 Go。Node.js 后端源码已从 Git 项目移除，紧急回滚副本单独保存在 `/opt/personal-portal-legacy-node`，正常生产流量不会经过 Node.js。

## 生产拓扑

- Caddy 负责 HTTPS、静态前端、上传文件与反向代理；公共静态站点配置 `nosniff`、禁止 iframe 嵌入、严格跨源 Referrer Policy 与限制摄像头/麦克风/定位的 Permissions Policy。
- 网站由 Cloudflare 代理时，Caddy 只信任 Cloudflare 官方 IPv4/IPv6 CIDR，并只从 `CF-Connecting-IP` 解析客户端地址；Go 只接受 Caddy 通过 `X-Portal-Client-IP` 注入的已解析地址。Cloudflare 更新 IP 段时应同步更新 Caddyfile 并验证，不要信任任意来源的转发头。
- /api/* 去除 /api 前缀后代理至 127.0.0.1:8081 的唯一 Go API。
- Caddy 明确拒绝公网访问 /api/events、/api/dbz、/api/readyz 和 /api/metrics。这些控制端点只供本机部署、健康检查与监控使用。
- Go 使用 database/sql + pgx，负责认证、会话、内容、历史版本、回收站、资源文件、公开内容互动、事件接收、分发、租约、重试和业务处理。
- C 事件核心仅使用 FreeBSD kqueue/kevent 和 Unix Socket 传送事件 ID/type 元数据；业务 payload、持久化、租约与处理结果只由 Go/PostgreSQL 处理。
- C IPC 协议为 v2。推送由 Go 使用 ACK 确认接收；PUBLISHED 和 ACKNOWLEDGED 都不是业务完成凭证。数据库状态是恢复和业务完成的唯一事实来源。
- Go 与 C 服务由 FreeBSD rc.d 管理，服务用户为 personalportal，socket 位于 /var/run/personal-portal/event.sock，权限为 0600。
- 日志位于 /var/log/personal-portal/；数据库连接 URL 存放于 /usr/local/etc/personal-portal/database-url，权限为 0640，不得提交到 Git 或打印到日志。

## 一键部署

部署脚本要求仓库干净且所有变更已经提交。脚本会执行 OpenAPI 校验与类型生成检查、前端构建和测试、Go race tests/vet/build、C 编译与 IPC 测试；然后创建完整 PostgreSQL 备份、执行未应用的迁移、安装 Go/C 二进制、切换静态前端与 Caddy 路由，最后运行生产健康检查。

    cd /opt/personal-portal
    git status --short
    ./deploy/freebsd/deploy.sh

部署过程会在 /var/backups/personal-portal/ 保存数据库 dump、旧前端 release 目标、旧 Go/C 二进制和旧 Caddyfile。部署成功后会停止并禁用 personal_portal_auth_api 服务；Caddy 不再将任何业务 API 转发到 8082。回滚脚本会临时将该服务指向 `/opt/personal-portal-legacy-node/backend/auth-api`。

**数据库迁移安全：** 发布前先生成并保留完整 PostgreSQL dump。迁移按版本顺序执行；包含显式 `BEGIN/COMMIT` 的迁移由自身控制事务，历史上没有显式事务的迁移由 `psql --single-transaction` 包裹，避免单个迁移中途失败留下半套 DDL。迁移采用向前兼容优先的策略，不在部署失败时自动执行反向 SQL，也不会自动恢复整库 dump；恢复数据库属于单独的人工恢复决策，必须先评估发布后新增数据是否会丢失。

**开发期保护规则：默认禁止自动回退。** `PERSONAL_PORTAL_RELEASE_MODE` 默认是 `development`；即使切换服务后健康检查失败，脚本也只输出诊断目录与手动回滚命令，不擅自切换回旧版本，并保留 `/var/tmp/personal-portal-deploy-STAMP` 诊断材料。只有进入稳定期后，由操作者显式设置 `PERSONAL_PORTAL_RELEASE_MODE=stable PERSONAL_PORTAL_AUTO_ROLLBACK=1`，才允许部署失败时自动回退；该条件已有独立 Shell 测试并纳入 GitHub CI。数据库备份与迁移前置检查仍然执行；自动回退不等于数据库反向迁移。

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

回滚会恢复旧 Go/C 二进制、Caddyfile 和前端 release，重启服务，并将 Node.js 回滚服务指向 `/opt/personal-portal-legacy-node` 中的归档副本。回滚不会自动恢复数据库，因为这会丢失备份时间点之后的新数据；如确实需要数据库恢复，应先评估影响并单独执行。

## 生产验证覆盖

CI 和目标主机测试覆盖：

- Go 单元测试、竞态检测、静态分析与 PostgreSQL 集成测试。
- 全量 SQL 迁移从空数据库按顺序执行。
- OpenAPI 规范 lint、TypeScript 生成结果漂移检查和 Vue/TypeScript 类型检查。
- C IPC v2 的版本校验、请求超时边界、ACK 匹配、重复/错误 ACK、单订阅连接有界 FIFO、并发客户端和 SIGTERM 退出。
- FreeBSD 上真实 C 核心重启后 Go 订阅自动重连并重新收到推送。
- 发布后的本机 readiness、公开 API、认证边界、内部路由隔离和前端 HTTP 状态检查。
