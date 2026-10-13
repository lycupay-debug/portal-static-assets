# FreeBSD 部署与服务管理

目标系统：FreeBSD 15.1-RELEASE-p4 / amd64。

## 服务布局

- 运行用户/组：`personalportal`，无交互登录权限。
- 二进制：`/usr/local/libexec/personal-portal/event-core`、`/usr/local/libexec/personal-portal/api`。
- C IPC socket：`/var/run/personal-portal/event.sock`，由 C 服务创建，权限 0600。
- PID 文件：`/var/run/personal-portal/event-core.pid`、`/var/run/personal-portal/api.pid`。
- 日志：`/var/log/personal-portal/event-core.log`、`/var/log/personal-portal/api.log`。
- API 默认仅监听 `127.0.0.1:8081`，不直接暴露到公网。
- 服务配置：`/usr/local/etc/rc.d/personal_portal_event_core`、`/usr/local/etc/rc.d/personal_portal_api`。

两个服务暂时使用同一个受限用户，因为当前 socket 权限为 0600；拆分用户需要另外设计组权限和 socket 访问控制。

## 首次安装（手动执行）

在仓库根目录执行：

```sh
cd /opt/personal-portal
make -C event-core clean all test
mkdir -p /usr/local/libexec/personal-portal
install -o root -g wheel -m 0555 event-core/event-core /usr/local/libexec/personal-portal/event-core
cd backend
gofmt127 -w api/*.go
go127 test -race ./...
go127 vet ./...
go127 build -o /usr/local/libexec/personal-portal/api ./api
chown root:wheel /usr/local/libexec/personal-portal/api
chmod 0555 /usr/local/libexec/personal-portal/api
pw useradd personalportal -d /nonexistent -s /usr/sbin/nologin -c "Personal Portal service" -w no
install -o root -g wheel -m 0555 /opt/personal-portal/deploy/freebsd/personal_portal_event_core /usr/local/etc/rc.d/personal_portal_event_core
install -o root -g wheel -m 0555 /opt/personal-portal/deploy/freebsd/personal_portal_api /usr/local/etc/rc.d/personal_portal_api
```

如果服务用户已存在，跳过 `pw useradd`。二进制安装前先运行测试；不要把仓库目录直接作为生产运行目录。

## 试运行与验证

先只启用 C 事件核心，再启动 API：

```sh
service personal_portal_event_core onestart
service personal_portal_api onestart
fetch -qo - http://127.0.0.1:8081/healthz
fetch -qo - http://127.0.0.1:8081/readyz
service personal_portal_api onestop
service personal_portal_event_core onestop
```

预期两个端点分别返回 `ok` 和 `ready`。停止 C 核心后，API 的 `/readyz` 应返回 HTTP 503。

确认服务启停、权限、日志、socket 清理和重启行为后，再通过 `/etc/rc.conf` 启用开机启动：

```sh
personal_portal_event_core_enable="YES"
personal_portal_api_enable="YES"
```

配置可放在 `/etc/rc.conf.d/personal_portal_event_core` 和 `/etc/rc.conf.d/personal_portal_api`，也可按系统习惯统一维护在 `/etc/rc.conf`。API 依赖 C 事件核心，需先启动核心。

## 运维边界

- 当前部署脚本不会自动删除遗留 socket；若启动报告 stale socket，应先确认对应进程不存在，再由管理员手动清理，避免误删活跃 socket。
- 目前尚未设置自动重启、资源限制、日志轮转和外部反向代理。
- `/healthz` 只检查 HTTP 进程；`/readyz` 检查 C 核心 IPC。
- 当前 IPC 仅支持 PING/STATUS，不是持久化业务事件队列。
- 初次验证通过前不要设置开机自启。

## 服务可靠性验证记录

以下生命周期与故障路径测试已在 FreeBSD 主机上执行：

- 两个服务均能正常启动和停止；测试结束后状态均为停止。
- API 执行服务重启后恢复正常，`/readyz` 返回 `ready`。
- 对 API 进程发送 `SIGKILL` 后，`service ... status` 能识别其已停止，PID 文件被清理；随后可以重新启动 API。
- 对 C 事件核心发送 `SIGKILL` 后，Unix socket 路径会残留。服务脚本拒绝在遗留 socket 存在时启动。确认没有 `event-core` 进程后，手动移除遗留 socket，核心即可重新启动。
- 保持 API 运行并停止 C 事件核心时，`/healthz` 仍返回 `ok`，`/readyz` 返回 HTTP 503。重新启动核心后，`/readyz` 恢复为 `ready`，无需重启 API。
- 正常停止后，PID 文件和 socket 均被清理。运行目录为 `personalportal:personalportal`、权限 `0750`；日志目录为 `root:wheel`、权限 `0750`；日志文件为 `root:wheel`、权限 `0600`。
- 服务脚本和已安装二进制均为 `root:wheel`、权限 `0555`。
- `/etc/rc.conf` 中没有启用这两个服务，开机自启仍处于关闭状态。

仍待处理的运维限制：C 核心遭遇 `SIGKILL` 后，需要管理员确认进程不存在，再手动删除遗留 socket；目前尚未配置自动进程监控/重启和日志轮转。这些测试验证的是服务生命周期与故障路径，不代表已具备公网生产环境的上线条件。
