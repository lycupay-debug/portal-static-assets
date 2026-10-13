# Backend API

## 当前实现

首个 Go 标准库 API 原型位于 `api/main.go`：

- `GET /healthz`：检查 HTTP 进程是否响应。
- `GET /readyz`：通过 Unix Domain Socket 请求 C 事件核心的 `STATUS`，仅在收到匹配的 `v1 <request_id> OK RUNNING` 响应时返回 200，否则返回 503。
- 默认监听 `127.0.0.1:8081`，不对外网开放。
- 可通过 `PERSONAL_PORTAL_HTTP_ADDR` 和 `PERSONAL_PORTAL_EVENT_SOCKET` 覆盖监听地址和 socket 路径。
- IPC 客户端设有连接/读写超时，并验证协议版本、请求 ID、状态和响应载荷。
- `api/main_test.go` 包含 IPC 成功响应和异常响应测试。

## 本地开发

需要 Go 1.24 或兼容的较新版本：

```sh
cd backend
gofmt -w api/*.go
go test -race ./...
go vet ./...
go run ./api
```

## FreeBSD 验证记录

在目标 FreeBSD 15.1-RELEASE-p4 amd64 主机上安装了 `go127` 包（Go 1.27.2，仅作为编译/验证工具），并完成以下检查：

- `gofmt127 -w api/main.go api/main_test.go`：完成。
- `go127 test -race ./...`：通过。
- `go127 vet ./...`：通过。
- `go127 build -o /var/tmp/personal-portal-api ./api`：通过，生成 FreeBSD amd64 可执行文件。
- IPC 集成测试：C 事件核心运行时，`GET /healthz` 返回 200、`GET /readyz` 返回 200；停止 C 事件核心后，`GET /readyz` 返回 503。
- API 安装至 `/usr/local/libexec/personal-portal/api`，并通过 FreeBSD `rc.d` 临时启动验证正式默认地址 `127.0.0.1:8081` 的 `/healthz` 与 `/readyz`。
- 两个服务的 `onestart`、`onestop` 和状态检查均已验证；测试结束后服务均已停止，未启用开机自启。

服务配置源文件位于 `deploy/freebsd/`，部署步骤见 `docs/operations/freebsd-deployment.md`。这些验证不代表业务事件投递、持久化或重试机制已实现。


## Durable event intake

`POST /events` accepts `{ "event_id": "<UUID>", "event_type": "lowercase.name", "payload": {} }` and persists it to `portal.event_inbox`. Apply `backend/migrations/002_event_inbox.sql` explicitly before deploying this API version. The endpoint guarantees durable receipt and idempotent duplicate handling only; event dispatch/processing is not implemented yet.
