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

测试覆盖：PING、STATUS、未知命令、协议版本、请求 ID、嵌入 NUL、超长请求、24 个并发客户端，以及 SIGTERM 退出。

## 本地协议 v1

每个请求一行，使用 ASCII 文本，以换行符结尾：

```text
v1 <request_id> <command>\n
```

支持命令：

- PING → v1 <request_id> OK PONG
- STATUS → v1 <request_id> OK RUNNING

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

默认路径为 /var/run/personal-portal/event.sock。部署前必须由部署步骤创建并正确授权 /var/run/personal-portal 目录；当前阶段尚未安装 rc.d 服务，也不会自动以 root 权限运行。

## 当前边界与后续事项

这个程序目前是事件核心的连通性原型，不是完整的业务任务调度器。它不提供持久化、重试、幂等、身份认证或跨进程任务队列。Unix socket 的文件权限是当前的访问控制边界。下一阶段应定义稳定的内部事件结构与 Go API 的 IPC 契约，并补上压力测试、资源清理验证和服务部署配置。
