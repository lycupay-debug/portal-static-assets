# Personal Portal

FreeBSD 原生部署的个人知识与工具门户。项目由 GitHub 管理，不使用 Docker。

## 当前技术方向

- 前端：Vue 3、TypeScript、Vite；生产环境只提供构建后的静态文件。
- 事件驱动核心：必须使用 C 语言实现，并优先利用 FreeBSD 原生 `kqueue` / `kevent`。
- 后端：Go API 使用 `database/sql` + pgx 连接 PostgreSQL；`POST /events` 持久化接收事件，后台分发器带租约和退避重试，通过 Unix Socket 通知 C 核心。
- 服务管理：FreeBSD `rc.d`。
- 内容：初期由项目自身管理，不接入 Notion。
- 依赖原则：优先标准库和系统原生能力；没有明确需求时不增加第三方服务。

## 目录

- `frontend/`：Vue + TypeScript 前端。
- `backend/api/`：Go HTTP API 与持久化事件接收入口。
- `event-core/`：C 语言 `kqueue` 事件核心；IPC v1 提供 PING/STATUS/EVENT 元数据通知。EVENT 的 ACCEPTED 不是业务处理完成确认。
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

当前已具备 FreeBSD 服务骨架、C IPC v1、Go API、PostgreSQL 事件收件箱、通知分发器，以及独立的 Go 业务处理器执行框架。`dispatch_state=notified` 只代表 C 核心接受了事件 ID/类型元数据；不代表 payload 已交给 C、事件已持久化在 C 侧或业务已完成。业务处理框架使用独立租约和重试状态，但当前没有注册生产业务 handler，因此尚无实际业务事件执行。生产服务仍保持停止且开机自启关闭。
