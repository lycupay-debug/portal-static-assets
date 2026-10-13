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
