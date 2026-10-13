# Backend API

## 当前实现

首个 Go 标准库 API 原型位于 `api/main.go`：

- `GET /healthz`：仅检查 HTTP 进程是否响应。
- `GET /readyz`：通过 Unix Domain Socket 请求 C 事件核心的 `STATUS`，仅在收到匹配的 `v1 <request_id> OK RUNNING` 响应时返回 200，否则返回 503。
- 默认监听 `127.0.0.1:8081`，不对外网开放。
- 通过 `PERSONAL_PORTAL_HTTP_ADDR` 和 `PERSONAL_PORTAL_EVENT_SOCKET` 可覆盖监听地址和 socket 路径。
- IPC 客户端设有连接/读写超时，并验证协议版本、请求 ID、状态和响应载荷。

## 本地开发

需要 Go 1.24 或兼容的较新版本：

```sh
cd backend
gofmt -w api/*.go
go test -race ./...
go vet ./...
go run ./api
```

CI 在 Ubuntu 上运行格式检查、测试和 vet。目标 FreeBSD 主机尚未安装/验证 Go 工具链，因此该 API 目前不能视为已在 FreeBSD 上构建或部署成功。下一步需先通过 CI，再在目标主机验证 Go 版本、编译、IPC 集成与运行方式。
