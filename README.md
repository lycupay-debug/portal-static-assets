# Personal Portal

FreeBSD 原生部署的个人知识与工具门户。项目由 GitHub 管理，不使用 Docker。

## 当前技术方向

- 前端：Vue 3、TypeScript、Vite；生产环境只提供构建后的静态文件。
- 事件驱动核心：必须使用 C 语言实现，并优先利用 FreeBSD 原生 `kqueue` / `kevent`。
- 后端：唯一业务 API 使用 Go（`database/sql` + pgx），统一处理认证、会话、内容、教学资源和事件生命周期；C 仅负责 FreeBSD 本地 IPC/kqueue 元数据通知，PostgreSQL 保存业务与事件持久状态。
- 服务管理：FreeBSD `rc.d`。
- 内容：初期由项目自身管理，不接入 Notion。
- 分类字段：文章内容独立保存父类与子类，年级与教育人力资源分类由共享 taxonomy 定义驱动。全站自定义下拉菜单统一使用 `VirtualizedSelect.vue`，设计参数通过 `frontend/src/monochrome.css` 中的 `--portal-select-*` CSS 变量集中维护。
- 依赖原则：优先标准库和系统原生能力；没有明确需求时不增加第三方服务。

## 目录

- `frontend/`：Vue + TypeScript 前端。
- `backend/api/`：Go HTTP API 与持久化事件接收入口。
- `event-core/`：C 语言 `kqueue` 事件核心；IPC v2 提供 PING/STATUS/PUBLISH/SUBSCRIBE/ACK 元数据通知。PUBLISHED 与 ACKNOWLEDGED 都不是业务处理完成确认。
- `deploy/freebsd/`：FreeBSD 部署与 `rc.d` 脚本。
- `docs/`：产品、架构、安全和运维文档。

## 本地构建前端

前端构建需要 Node.js 24 系列及 npm；生产业务 API 不依赖 Node.js。后端需要 Go 1.24+ 与 PostgreSQL。

```sh
cd frontend
npm ci
npm run type-check
npm run build
```

生产构建产物位于 `frontend/dist/`，不应将其作为源代码提交。

## 当前状态

当前架构目标为单一 Go 业务 API、PostgreSQL 事件收件箱/租约/重试/结果与 FreeBSD 原生 C `kqueue` 事件核心。C IPC v2 的 `PUBLISH` / `ACK` 只确认通知扇出与接收，不代表业务完成；Go 是业务 payload 的唯一处理者，PostgreSQL 是恢复依据。CI 覆盖 OpenAPI 生成契约、全量迁移、Go race tests、C IPC FIFO/ACK/错误 ACK 与并发测试。生产部署使用 deploy/freebsd/deploy.sh，包含备份、迁移、健康检查与可回滚发布。
