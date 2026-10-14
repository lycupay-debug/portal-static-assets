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
install -d -o root -g wheel -m 0755 /usr/local/etc/newsyslog.conf.d
install -o root -g wheel -m 0644 /opt/personal-portal/deploy/freebsd/newsyslog/personal-portal.conf /usr/local/etc/newsyslog.conf.d/personal-portal.conf
```

如果服务用户已存在，跳过 `pw useradd`。二进制安装前先运行测试；不要把仓库目录直接作为生产运行目录。

## PostgreSQL 数据库

- 复用主机 PostgreSQL 17，数据库为 `personal_portal`，应用角色为 `personal_portal_app`。
- `n8n_db`、`host_mcp_oauth` 与 `personal_portal` 相互隔离；应用角色仅获准连接 `personal_portal`。
- 数据库连接 URL 存放在 `/usr/local/etc/personal-portal/database-url`，属主 `root:personalportal`，权限 `0640`；不得提交到 Git、写入日志或复制到公开构建产物。
- `pg_hba.conf` 对 `personal_portal_app` 的本地 Unix socket、IPv4 loopback 和 IPv6 loopback 连接使用 SCRAM-SHA-256；其他既有角色规则保持原样。
- Go API 使用 pgx 的 `database/sql` 驱动与有限连接池。`/dbz` 检查数据库连通性；`/readyz` 同时检查 C 核心与数据库。
- 数据库迁移文件位于 `backend/migrations/`。业务迁移应由管理员/部署步骤显式执行，不在 API 启动时自动执行。
- `portal.schema_migrations` 记录迁移版本；迁移 002 创建 `portal.event_inbox`，迁移 003 增加通知分发状态、租约和重试字段，迁移 004 增加业务处理租约及下一次重试时间，迁移 006 创建账户与会话表，迁移 007 创建门户内容表。内容 API 说明见 `backend/content-api/README.md`。

应用基础迁移时，避免把包含密码的 URL 放在命令行参数中。可使用 Python 将密码放入子进程环境，再调用 `psql`：

```python
import os, subprocess
from urllib.parse import urlparse, unquote

url = open("/usr/local/etc/personal-portal/database-url", encoding="utf-8").read().strip()
parsed = urlparse(url)
env = os.environ.copy()
env["PGPASSWORD"] = unquote(parsed.password or "")
subprocess.run([
    "/usr/local/bin/psql", "-X", "-v", "ON_ERROR_STOP=1",
    "-h", parsed.hostname, "-p", str(parsed.port or 5432),
    "-U", unquote(parsed.username or ""), "-d", parsed.path.lstrip("/"),
    "-f", "/opt/personal-portal/backend/migrations/001_portal_foundation.sql",
], env=env, check=True)
```

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

- 两个服务由 FreeBSD 原生 `daemon` 监督，子进程异常退出后默认等待 2 秒再重启。可分别通过 `personal_portal_event_core_restart_delay` 和 `personal_portal_api_restart_delay` 调整重启间隔。
- rc.d 控制使用 supervisor PID 文件，避免向受监督的子进程直接发送停止信号、继而触发自动重启。子进程 PID 文件仅用于诊断。
- 首次启动若发现遗留 socket 且没有运行中的 `event-core` 进程，脚本会拒绝自动删除；管理员应确认没有活跃进程后再清理。受监督的核心进程异常退出后，重启的 C 程序会在绑定前清理其 socket 路径。
- 日志轮转配置安装在 `/usr/local/etc/newsyslog.conf.d/personal-portal.conf`。日志达到 1 MiB 时轮转，保留 7 份压缩归档，并向 daemon supervisor 发送 `SIGHUP`，使其重新打开日志文件。归档与当前日志权限为 `0600`。
- 尚未配置资源限制、外部反向代理或 HTTPS；当前 API 仅监听 `127.0.0.1:8081`。
- `/healthz` 只检查 HTTP 进程；`/dbz` 检查 PostgreSQL；`/readyz` 同时检查 C 核心 IPC 与 PostgreSQL。
- C IPC v1 支持 PING/STATUS/EVENT 元数据通知。Go API 将事件持久化后由后台分发器领取，通过 Unix Socket 发送 ID/type 元数据；失败时退避重试，租约过期后可恢复。`notified` 只表示 C 返回 `ACCEPTED`，不代表 C 持久化了事件、收到了 payload 或完成业务处理。
- 开机自启仍保持关闭，直到完成后续部署与安全检查。

## 服务可靠性验证记录

以下测试已在 FreeBSD 15.1 主机上执行：

- 两个服务均可启动、停止；停止后 supervisor/child PID 文件、socket 和相关进程均清理完毕。
- API 正常重启后 `/readyz` 返回 `ready`。
- 对 API 子进程发送 `SIGKILL` 后，daemon 在设定间隔后启动新子进程；API 的 `/healthz` 和 `/readyz` 随后恢复正常。
- 对 C 核心子进程发送 `SIGKILL` 后，daemon 启动新子进程；新核心清理遗留 socket 并恢复监听，API `/readyz` 恢复为 `ready`。
- 保持 API 运行并正常停止 C 核心时，`/healthz` 返回 `ok`、`/readyz` 返回 HTTP 503；重新启动核心后，`/readyz` 恢复为 `ready`。
- 使用 `newsyslog` 强制轮转两份日志成功；已创建压缩归档，当前日志 inode 已变化，daemon 重新打开日志文件，轮转后健康检查仍正常。
- 已执行主机重启验证：C 核心与 API 均按 rcorder 顺序自动启动，`/healthz` 返回 `ok`、`/readyz` 返回 `ready`；随后将自启动配置恢复为 `NO`。
- 日志文件和归档均为 `root:wheel`、权限 `0600`；服务脚本和已安装二进制均为 `root:wheel`、权限 `0555`。
- `/etc/rc.conf` 中未启用两个服务，开机自启仍关闭。

这些测试验证了服务生命周期、子进程自动恢复和日志轮转；不代表网站已具备公网生产上线条件。尚未配置反向代理、HTTPS、资源限制或业务事件持久化。


## 事件收件迁移

在应用迁移 001 后，显式应用迁移 002：

```sh
# Use the same protected URL-file parsing procedure shown above;
# replace the migration path with backend/migrations/002_event_inbox.sql.
```

迁移不会由 API 自动执行。部署新 API 前先备份数据库并依次应用迁移 002、003；回滚代码不应自动删除已收到的事件数据。

迁移 003 文件：`backend/migrations/003_event_dispatch_state.sql`，已在当前 `personal_portal` 数据库应用并记录为版本 3。迁移 004 文件：`backend/migrations/004_event_processing_leases.sql`，已在当前数据库应用并记录为版本 4。
