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

当前 FreeBSD 主机尚未安装/验证 Go 工具链，因此 Go 代码目前尚未实际编译或运行测试，也没有部署 API 服务。必须先在可用的 Go 环境中完成格式检查、单元测试和 vet，再在目标 FreeBSD 主机验证工具链、IPC 集成与运行方式。
