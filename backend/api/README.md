# Backend API

## 当前实现

首个 Go 标准库 API 原型位于 `api/main.go`：

- `GET /healthz`：检查 HTTP 进程是否响应。
- `GET /readyz`：通过 Unix Domain Socket 请求 C 事件核心的 `STATUS`，仅在收到匹配的 `v1 <request_id> OK RUNNING` 响应时返回 200，否则返回 503。
- 默认监听 `127.0.0.1:8081`，不对外网开放。收到 SIGTERM 或中断信号时，HTTP 服务执行最长 5 秒的优雅关闭，并取消后台分发、订阅和处理循环；未完成事件由 PostgreSQL 租约恢复机制接管。
- 可通过 `PERSONAL_PORTAL_HTTP_ADDR` 和 `PERSONAL_PORTAL_EVENT_SOCKET` 覆盖监听地址和 socket 路径。
- IPC 客户端设有连接/读写超时，并验证协议版本、请求 ID、状态和响应载荷。
- `POST /events` 将事件持久化到 `portal.event_inbox`；后台 `EventDispatcher` 使用 `FOR UPDATE SKIP LOCKED` 领取待通知事件，租约超时后可恢复，并在 IPC 失败时以 1–64 秒指数退避重试。分发状态更新以 `dispatch_attempts` 作为租约 fencing token，旧领取者不能覆盖新领取者的通知或重试结果。
- IPC `EVENT` 仅传送 event ID/type 元数据。数据库 `dispatch_state=notified` 不代表业务完成，也不应修改事件的业务 `status`。
- `api/event_processor.go` 提供独立业务处理执行层：默认启动 4 个业务 Worker（`PERSONAL_PORTAL_EVENT_WORKERS` 可设为 1–8，超出上限会限制为 8）；各 Worker 使用独立 ID 并通过 `FOR UPDATE SKIP LOCKED`、处理租约竞争领取事件。成功后才写 `completed`；失败时指数退避重试，达到 10 次后写 `failed`；过期的 `processing` 租约可被重新领取。
- 处理器必须遵守幂等约定、响应 context 超时；由于外部副作用与数据库完成状态无法原子提交，执行语义是至少一次，处理器不得假设只执行一次。
- 只有 `portal.content.validate` 注册了生产业务处理器；其他未知事件类型不会被自动标为完成或失败。新增事件类型时，必须先定义业务语义并注册对应 handler。
- 迁移 `004_event_processing_leases.sql` 为处理租约和重试调度增加字段；迁移 `005_event_processing_result.sql` 增加处理结果字段。`PERSONAL_PORTAL_TEST_DATABASE_URL_FILE` 可显式启用 PostgreSQL 集成测试；应优先指向专用测试库。
- `api/main_test.go` 包含 IPC 状态、事件通知成功/异常响应及 HTTP 接收幂等测试。

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

`POST /events` accepts `{ "event_id": "<UUID>", "event_type": "lowercase.name", "payload": {} }` and persists it to `portal.event_inbox`. Apply migrations 002, 003, 004, and 005 explicitly before deploying this API version. The endpoint guarantees durable receipt and idempotent duplicate handling. The background dispatcher sends a metadata notification and retries failures; it does not claim business processing or completion. The production handler registry currently includes `portal.content.validate`, which validates portal content fields and persists a JSON result. Verify the end-to-end path after deployment by posting a unique event ID and checking that `portal.event_inbox.status` becomes `completed` and `processing_result` is populated. The C core accepts only event ID/type metadata; business payload processing remains in Go.
