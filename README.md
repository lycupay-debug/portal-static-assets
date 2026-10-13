# Personal Portal

FreeBSD 原生部署的个人知识与工具门户。项目由 GitHub 管理，不使用 Docker。

## 当前技术方向

- 前端：Vue 3、TypeScript、Vite；生产环境只提供构建后的静态文件。
- 事件驱动核心：必须使用 C 语言实现，并优先利用 FreeBSD 原生 `kqueue` / `kevent`。
- 后端：Go API 使用 `database/sql` + pgx 连接 PostgreSQL；`POST /events` 已实现持久化事件接收与按 UUID 幂等。
- 服务管理：FreeBSD `rc.d`。
- 内容：初期由项目自身管理，不接入 Notion。
- 依赖原则：优先标准库和系统原生能力；没有明确需求时不增加第三方服务。

## 目录

- `frontend/`：Vue + TypeScript 前端。
- `backend/api/`：Go HTTP API 与持久化事件接收入口。
- `event-core/`：C 语言 `kqueue` 事件核心；当前 IPC v1 提供 PING/STATUS，业务事件分发尚未接入。
- `deploy/freebsd/`：FreeBSD 部署与 `rc.d` 脚本。
- `docs/`：产品、架构、安全和运维文档。

## 本地构建前端

要求 Node.js 24 系列及 npm。

```sh
cd frontend
npm ci
npm run type-check
npm run build
```

生产构建产物位于 `frontend/dist/`，不应将其作为源代码提交。

## 当前状态

当前已具备 FreeBSD 服务骨架、C IPC v1、Go API、PostgreSQL 基础迁移及 `portal.event_inbox` 持久化收件表。`POST /events` 只保证事件被数据库接收，不代表 C 核心已调度或处理事件。生产服务开机自启仍保持关闭。
