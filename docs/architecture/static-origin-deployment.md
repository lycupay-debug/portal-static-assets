# 独立静态资源子域名部署方案

## 状态与边界

- 目标静态资源域名：`https://assets.lycupaynotion.top`。
- 门户 HTML、路由和 API 继续由现有主站提供；静态资源域名不承载门户 HTML，不代理 API，也不需要会话 Cookie。
- 当前阶段只允许在独立仓库中构建、校验和打包候选版本。
- 未经单独确认，不修改 DNS/Cloudflare、生产 Caddyfile、生产发布目录或门户线上引用，不重启服务。

## URL/base 策略

使用 Vite CLI 的显式绝对 base：

    cd /opt/portal-static-assets/frontend
    npm run build:static-origin

该命令构建到独立目录 frontend/dist-static-origin，不覆盖常规 frontend/dist。HTML 中的入口 JS/CSS、JS 运行时的延迟加载 chunk，以及图标 CSS 中的 /icons/... 引用应统一指向 https://assets.lycupaynotion.top/。

API 和门户路由继续保持主站根路径（例如 /api/...、/knowledge）；不能把 index.html 放到资源子域名作为页面入口，否则根相对 API 请求会错误地发往资源子域名。

## 发布包策略

npm run package:static-origin 生成独立资源包。包只包含：

- assets/：哈希命名的 JS、CSS 与其他构建资源。
- icons/：本地 SVG 图标。
- 存在时的 favicon.ico 和 THIRD_PARTY_NOTICES.txt。

包明确排除 index.html，避免静态资源服务意外成为门户文档源。包内 manifest 记录源提交、基线提交、静态域名、文件大小与 SHA-256；压缩包附带 SHA-256 sidecar。发布器拒绝未提交的跟踪文件变更、符号链接和覆盖已有产物。

## 建议的生产路由约束（尚未应用）

在正式部署前，需要先核实 Cloudflare DNS、当前源站 IP、TLS 模式、Caddy 配置验证能力与现有站点路由。拟为 assets.lycupaynotion.top 建立独立站点块：

1. 只允许读取 /assets/*、/icons/*，以及经确认需要的静态文件；对 /、/index.html、/api/*、未知路径返回 404，不配置 SPA fallback，也不启用目录浏览。默认静态文件服务器在无 index.html 时可能展示目录列表，必须显式阻止。
2. 只读静态目录应与门户生产发布目录分离，例如独立版本目录加只读 current 指针；候选包先解压至新版本目录，校验 manifest 后再切换。
3. 为跨源 ES modules 返回精确的 Access-Control-Allow-Origin: https://www.lycupaynotion.top 与 Vary: Origin；不启用 credentialed CORS，不设置资源域 Cookie。若实际门户规范域名不同，必须先确认再配置。
4. 为哈希命名的 /assets/* 设置长缓存与 immutable；未哈希图标使用较短缓存或后续引入内容哈希命名。确认 MIME 类型、压缩和 X-Content-Type-Options: nosniff。
5. 在切换主站 HTML 引用前，分别测试 JS module CORS、CSS、SVG mask 图标、延迟加载编辑器 chunk、错误状态与浏览器控制台。CORS 不应放宽为任意来源。
6. 配置变更前备份 Caddyfile 并执行语法/适配校验；只有在用户再次授权后才能应用生产配置或重载服务。

## 生产环境只读盘点（2026-10-28）

- Cloudflare 中 lycupaynotion.top zone 处于 active 状态。
- 查询 assets.lycupaynotion.top 的 DNS 记录返回 0 条；尚无该子域名记录。
- www.lycupaynotion.top 当前为代理开启的 A 记录，指向 54.66.101.177。
- 当前 Caddyfile 仅包含 www.lycupaynotion.top 与 go.lycupaynotion.top 站点块；没有 assets.lycupaynotion.top 站点块。
- 上述检查仅读取现状；未创建 DNS 记录、未编辑 Caddyfile，也未重载服务。

因此正式部署至少需要新增 Cloudflare DNS 记录和独立 Caddy 站点路由。这两项均属于生产配置变更，必须在另行汇报并取得授权后执行。

## 当前验证

- build:static-origin：使用正式候选 base 在独立输出目录构建，并运行类型检查、bundle budget 和静态源校验。
- check-static-origin-build.mjs：检查 HTML 入口 URL、资源文件存在性、图标绝对 URL、JS 延迟 chunk 的静态域名及主站 API 根路径。
- package:static-origin：只打包资源，不包含门户 HTML。
- 额外需要通过本地 HTTP smoke test 检查资源 MIME/响应头；正式域名 CORS/DNS/TLS 验证须等生产配置获批后进行。

## 已执行部署记录（2026-10-28）

- Cloudflare DNS record ID：`6bc94cb854f059b8aedb2b727f8e2a67`。
- 新增记录：`A assets.lycupaynotion.top -> 54.66.101.177`，Cloudflare proxy enabled，TTL automatic。
- Cloudflare zone SSL mode 为 `strict`，Always Use HTTPS 为 `on`；公共 DNS 已解析至 Cloudflare 代理地址。
- 发布目录：`/usr/local/www/portal-static-assets/releases/75da6fdd80d1`；`current` 指向该版本目录。所有文件在发布前通过 manifest SHA-256/大小校验，目录/文件权限分别为 0755/0644。
- Caddy 配置备份：`/usr/local/etc/caddy/Caddyfile.pre-assets-20261028`。当前 Caddyfile 增加独立 `assets.lycupaynotion.top` 站点块；使用 `service caddy reload` 平滑加载，没有重启进程，Caddy PID 仍为 1461。
- Caddy 站点只处理 `/assets/*` 和 `/icons/*`，其余路径显式 404。跨源响应只允许 `https://www.lycupaynotion.top`；哈希资源使用一年 immutable 缓存，SVG 图标缓存 1 天。
- 线上验证：56 个实际公开的 assets/icons 文件均返回 HTTP 200，大小和 SHA-256 与 manifest 完全一致，CORS 响应头匹配；JS/CSS/SVG MIME 类型正确。
- 根路径、`/index.html`、`/api/health`、未知路径、`/favicon.ico`、`/THIRD_PARTY_NOTICES.txt` 和 `/icons/` 均返回 404；目录浏览未暴露。
- `https://www.lycupaynotion.top/` 仍返回原主站 HTML，资源地址仍为同源 `/assets/...`；尚未切换门户资源引用。
- 生产门户 Git 工作区未修改。主站 HTML 资源切换与浏览器端实际页面回归测试仍是单独阶段。

## 回滚原则

资源域名出现问题时，先恢复 `/usr/local/etc/caddy/Caddyfile.pre-assets-20261028` 并执行 Caddy 配置校验，再经授权平滑 reload；如需撤销 DNS，删除对应 DNS record ID `6bc94cb854f059b8aedb2b727f8e2a67`。当前主站 HTML 未引用资源子域名，因此移除静态站点不会要求门户回滚。不要自动重启服务。