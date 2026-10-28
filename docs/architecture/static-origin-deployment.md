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

1. 只允许读取 /assets/*、/icons/*，以及经确认需要的静态文件；对 /、/index.html、/api/*、未知路径返回 404，不配置 SPA fallback。
2. 只读静态目录应与门户生产发布目录分离，例如独立版本目录加只读 current 指针；候选包先解压至新版本目录，校验 manifest 后再切换。
3. 为跨源 ES modules 返回精确的 Access-Control-Allow-Origin: https://www.lycupaynotion.top 与 Vary: Origin；不启用 credentialed CORS，不设置资源域 Cookie。若实际门户规范域名不同，必须先确认再配置。
4. 为哈希命名的 /assets/* 设置长缓存与 immutable；未哈希图标使用较短缓存或后续引入内容哈希命名。确认 MIME 类型、压缩和 X-Content-Type-Options: nosniff。
5. 在切换主站 HTML 引用前，分别测试 JS module CORS、CSS、SVG mask 图标、延迟加载编辑器 chunk、错误状态与浏览器控制台。CORS 不应放宽为任意来源。
6. 配置变更前备份 Caddyfile 并执行语法/适配校验；只有在用户再次授权后才能应用生产配置或重载服务。

## 当前验证

- build:static-origin：使用正式候选 base 在独立输出目录构建，并运行类型检查、bundle budget 和静态源校验。
- check-static-origin-build.mjs：检查 HTML 入口 URL、资源文件存在性、图标绝对 URL、JS 延迟 chunk 的静态域名及主站 API 根路径。
- package:static-origin：只打包资源，不包含门户 HTML。
- 额外需要通过本地 HTTP smoke test 检查资源 MIME/响应头；正式域名 CORS/DNS/TLS 验证须等生产配置获批后进行。

## 回滚原则

此阶段无生产改动，因此无需生产回滚。将来正式接入时，主站 HTML 的资源 URL 切换应作为独立变更；回滚只恢复主站上一版 HTML/资源引用。不要因为资源子域名失败而自动修改或重启生产服务。