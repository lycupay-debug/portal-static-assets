# C Event Core

FreeBSD 原生 C 事件服务的最小可运行版本。使用 kqueue / kevent 监听 Unix Domain Socket、客户端读写和 SIGINT/SIGTERM。

## 当前实现

- C11，使用系统调用和标准库，不依赖第三方 C 库。
- 非阻塞 Unix Domain Socket；每个连接只处理一个请求。
- 最多 64 个并发客户端；超过上限的新连接会被关闭。
- 请求缓冲区上限 255 字节；响应缓冲区固定长度。
- 请求必须包含协议版本；拒绝无效版本、无效请求 ID、嵌入 NUL 和未知命令。
- socket 文件权限设为当前服务用户可读写（0600）。
- SIGINT / SIGTERM 由 EVFILT_SIGNAL 交给事件循环处理并执行清理。

## 构建与测试

在 FreeBSD 主机上运行：

```sh
cd /opt/personal-portal/event-core
make clean all
make test
```

C 测试覆盖：PING、STATUS、EVENT 元数据校验、SUBSCRIBE/PUBLISH 推送、未知命令、协议版本、请求 ID、嵌入 NUL、超长请求、24 个并发客户端，以及 SIGTERM 退出。Go 集成测试还会编译临时 C 核心进程，验证核心真实重启后订阅客户端能够重连并再次收到推送。

## 本地协议 v1

每个请求一行，使用 ASCII 文本，以换行符结尾：

```text
v1 <request_id> <command>\n
```

支持命令：

- PING → v1 <request_id> OK PONG
- STATUS → v1 <request_id> OK RUNNING
- EVENT <event_id> <event_type> → v1 <request_id> OK ACCEPTED（仅表示元数据校验通过）
- SUBSCRIBE → v1 <request_id> OK SUBSCRIBED，连接保持打开并接收推送
- PUBLISH <event_id> <event_type> → v1 <request_id> OK PUBLISHED；向当前订阅者广播 `v1 <push_id> EVENT <event_id> <event_type>`。这是 best-effort 通知，不提供持久投递保证。

错误响应形如：

```text
v1 <request_id> ERROR <error_code>
```

当前错误码包括 BAD_REQUEST、REQUEST_TOO_LARGE 和 UNKNOWN_COMMAND。服务在响应后关闭该客户端连接。请求 ID 仅允许 ASCII 字母、数字、连字符和下划线。

## 启动

开发测试可指定 socket 路径：

```sh
./event-core /var/tmp/personal-portal-event.sock
```

默认路径为 `/var/run/personal-portal/event.sock`。生产环境已安装 `rc.d` 服务 `personal_portal_event_core`，由专用 `personalportal` 用户运行，并通过 `/usr/sbin/daemon` 监督重启。运行目录由服务启动前置步骤创建，socket 权限为 0600。使用 `service personal_portal_event_core status` 检查服务。

## 当前边界与后续事项

这个程序负责 IPC 通知，不是持久消息队列或业务任务执行器。事件的持久事实、领取、租约、重试和幂等由 PostgreSQL 与 Go API 负责；C 核心推送只用于低延迟唤醒。订阅断开期间的推送可能丢失，因此 Go API 必须保留数据库恢复扫描。当前 IPC 访问控制依赖 Unix socket 文件权限（0600）及服务用户隔离；协议本身不提供身份认证。后续重点是持续验证真实进程重启、通知丢失恢复、租约恢复与业务副作用幂等。
