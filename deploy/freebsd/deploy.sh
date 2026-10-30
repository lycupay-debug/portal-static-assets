#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root." >&2
  exit 1
fi

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
bin_dir="/usr/local/libexec/personal-portal"
www_base="/usr/local/www/personal-portal"
static_www_base="/usr/local/www/portal-static-assets"
caddy_file="/usr/local/etc/caddy/Caddyfile"
backup_dir="/var/backups/personal-portal"
go_bin="${PERSONAL_PORTAL_GO_BIN:-}"
if [ -z "$go_bin" ]; then
  go_bin=$(command -v go || command -v go127 || true)
fi
if [ -z "$go_bin" ] || [ ! -x "$go_bin" ]; then
  echo "Go toolchain not found; set PERSONAL_PORTAL_GO_BIN to the Go compiler path." >&2
  exit 1
fi
stamp=$(date -u +%Y%m%dT%H%M%SZ)
work="/var/tmp/personal-portal-deploy-$stamp"
deployment_changed=0
. "$repo/deploy/freebsd/deploy-policy.sh"
personal_portal_require_service_restart_authorization
release_mode="${PERSONAL_PORTAL_RELEASE_MODE:-development}"
mkdir -p "$work" "$backup_dir"
chmod 0700 "$work" "$backup_dir"

rollback_on_failure() {
  rc=$?
  if [ "$rc" -ne 0 ]; then
    if [ "$deployment_changed" -eq 1 ]; then
      if personal_portal_auto_rollback_enabled; then
        echo "Deployment failed; stable mode explicitly enables automatic rollback (stamp $stamp)." >&2
        "$repo/deploy/freebsd/rollback.sh" "$stamp" || echo "Automatic rollback failed; manual recovery may be required: $repo/deploy/freebsd/rollback.sh $stamp" >&2
      else
        echo "Automatic rollback suppressed (release mode: $release_mode). Development deployments never auto-rollback." >&2
        echo "Inspect service state and deployment artifacts before deciding whether to roll back." >&2
        echo "Manual rollback command: $repo/deploy/freebsd/rollback.sh $stamp" >&2
      fi
    fi
    echo "Deployment diagnostics preserved at: $work" >&2
  else
    rm -rf "$work"
  fi
  exit "$rc"
}
trap rollback_on_failure EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# The current production stack is Go + C event core. Rollback uses the
# pre-cutover Go binaries, Caddyfile, and frontend release captured below;
# the retired Node.js API is not a rollback dependency and stays disabled.

if [ -n "$(git -C "$repo" status --porcelain)" ]; then
  echo "Repository must be clean and committed before deployment." >&2
  exit 1
fi

# Deployment-time Caddy configuration guard (kept inline; standalone test files are removed).
config="$repo/deploy/freebsd/Caddyfile"
for directive in \
  'X-Content-Type-Options "nosniff"' \
  'X-Frame-Options "DENY"' \
  'Referrer-Policy "strict-origin-when-cross-origin"' \
  'Permissions-Policy "camera=(), microphone=(), geolocation=()"' \
  'trusted_proxies static' \
  'trusted_proxies_strict' \
  'client_ip_headers CF-Connecting-IP' \
  'handle /uploads/* {' \
  'redir https://{http.request.host}/api{uri} 308' \
  'handle /api/uploads/* {' \
  'uri strip_prefix /api' \
  'reverse_proxy 127.0.0.1:8081 {' \
  'header_up X-Portal-Client-IP {client_ip}' \
  'handle /editor {' \
  'redir https://{http.request.host}/content-management 308' \
  'handle /editor/assets/* {' \
  'respond 410' \
  'handle_path /editor/* {' \
  'redir https://{http.request.host}/content-management{path} 308'; do
  if ! grep -Fq "$directive" "$config"; then
    echo "Missing required Caddy deployment directive: $directive" >&2
    exit 1
  fi
done
if ! grep -Fq 'go.lycupaynotion.top {' "$config" || ! grep -Fq 'reverse_proxy 127.0.0.1:8766' "$config"; then
  echo "Caddy deployment configuration must preserve the GSM reverse proxy." >&2
  exit 1
fi

cd "$repo/frontend"
npm ci
python3.14 "$repo/scripts/compliance/scan_assets.py" --repo "$repo" --release "$www_base/current" --out "$work/compliance-preflight" --strict-licenses
npm run api:lint
npm run api:check
npm run type-check
npm run build:static-origin
npm run package:static-origin

cd "$repo/backend"
"$go_bin" vet ./...
"$go_bin" build -trimpath -ldflags="-s -w" -o "$work/api" ./api

cd "$repo/event-core"
make clean all

# Create a full PostgreSQL backup before applying migrations or replacing services.
dump_tmp="/tmp/personal-portal-pre-go-cutover-$stamp.dump"
su -m postgres -c "pg_dump -Fc -d personal_portal -f '$dump_tmp'"
install -o root -g wheel -m 0600 "$dump_tmp" "$backup_dir/pre-go-cutover-$stamp.dump"
rm -f "$dump_tmp"

cp "$bin_dir/api" "$bin_dir/api.pre-go-cutover-$stamp"
cp "$bin_dir/event-core" "$bin_dir/event-core.pre-go-cutover-$stamp"
cp "$caddy_file" "$caddy_file.pre-go-cutover-$stamp"
readlink "$www_base/current" > "$backup_dir/frontend-current-$stamp"
readlink "$static_www_base/current" > "$backup_dir/static-current-$stamp"
chmod 0600 "$backup_dir/static-current-$stamp"
chmod 0600 "$backup_dir/frontend-current-$stamp"

# Retire the legacy root executor only after a full backup and only with explicit operator authorization.
legacy_executor_enabled=$(sysrc -n personal_portal_task_executor_enable 2>/dev/null || echo NO)
legacy_executor_process=0
if pgrep -f '[/]usr/local/libexec/personal-portal/api --task-executor' >/dev/null 2>&1; then
  legacy_executor_process=1
fi
if [ -f /usr/local/etc/rc.d/personal_portal_task_executor ] || [ "$legacy_executor_enabled" = "YES" ] || [ "$legacy_executor_process" = "1" ]; then
  if [ "${PERSONAL_PORTAL_RETIRE_COMMAND_EXECUTOR:-0}" != "1" ]; then
    echo "Legacy root command executor detected. Re-run with PERSONAL_PORTAL_RETIRE_COMMAND_EXECUTOR=1 to authorize its decommission before migration 031." >&2
    exit 1
  fi
  "$repo/deploy/freebsd/retire-command-executor.sh"
fi

# Forward-only migrations run after the full backup. Migration 031 removes the retired task tables.
PERSONAL_PORTAL_MIGRATION_BACKUP="$backup_dir/pre-go-cutover-$stamp.dump" "$repo/deploy/freebsd/migrate.sh"

release="$www_base/releases/${stamp}-go-backend"
mkdir -p "$release"
cp -R "$repo/frontend/dist-static-origin/." "$release/"
if [ ! -f "$release/favicon.ico" ] && [ -f "$www_base/current/favicon.ico" ]; then
  cp "$www_base/current/favicon.ico" "$release/favicon.ico"
fi

# Prepare an immutable static-origin release from the exact same frontend build.
static_release="$static_www_base/releases/${stamp}-static-origin"
mkdir -p "$static_release/site"
for asset_dir in assets icons; do
  if [ -d "$repo/frontend/dist-static-origin/$asset_dir" ]; then
    cp -R "$repo/frontend/dist-static-origin/$asset_dir" "$static_release/site/"
  fi
done
chmod -R a+rX "$static_release"

deployment_changed=1
service personal_portal_api stop
service personal_portal_event_core stop
install -o root -g wheel -m 0555 "$work/api" "$bin_dir/api"
install -o root -g wheel -m 0555 "$repo/event-core/event-core" "$bin_dir/event-core"
install -d -o personalportal -g personalportal -m 0750 "$repo/frontend/dist"
install -d -o personalportal -g personalportal -m 0700 "$repo/frontend/node_modules/.tmp" "$repo/frontend/node_modules/.cache"
chown personalportal:personalportal "$repo/frontend/dist" "$repo/frontend/node_modules/.tmp" "$repo/frontend/node_modules/.cache"
chmod 0750 "$repo/frontend/dist"
chmod 0700 "$repo/frontend/node_modules/.tmp" "$repo/frontend/node_modules/.cache"
service personal_portal_event_core start
service personal_portal_api start

# Service supervisors return before the child process has necessarily bound
# its socket. Wait for actual readiness instead of racing the first request.
wait_for_url() {
  url="$1"
  pattern="$2"
  attempt=0
  while [ "$attempt" -lt 30 ]; do
    if body=$(curl -fsS --max-time 3 "$url" 2>/dev/null) && printf '%s' "$body" | grep -q "$pattern"; then
      return 0
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
  echo "Timed out waiting for readiness: $url" >&2
  curl -sS --max-time 3 "$url" >&2 || true
  return 1
}

# Check the new Go+C+PostgreSQL stack before moving public traffic.
wait_for_url http://127.0.0.1:8081/healthz '"status":"ok'
wait_for_url http://127.0.0.1:8081/dbz '"status":"ok'
wait_for_url http://127.0.0.1:8081/readyz '"status":"ready'
wait_for_url http://127.0.0.1:8081/metrics 'personal_portal_events'


install -o root -g wheel -m 0644 "$repo/deploy/freebsd/Caddyfile" "$caddy_file"
caddy validate --config "$caddy_file"
service caddy reload
ln -s "$static_release" "$static_www_base/current.next"
mv -h -f "$static_www_base/current.next" "$static_www_base/current"
ln -s "$release" "$www_base/current.next"
mv -h -f "$www_base/current.next" "$www_base/current"

legacy_auth_enabled=$(sysrc -n personal_portal_auth_api_enable 2>/dev/null || echo NO)
if [ "$legacy_auth_enabled" = "YES" ]; then
  service personal_portal_auth_api stop
else
  echo "Legacy Node auth API is already disabled; skipping stop."
fi
sysrc personal_portal_auth_api_enable=NO
"$repo/deploy/freebsd/health-check.sh"
deployment_changed=0
echo "Deployment completed. Stamp: $stamp"
echo "Database backup: $backup_dir/pre-go-cutover-$stamp.dump"
echo "Previous frontend release: $(cat "$backup_dir/frontend-current-$stamp")"
echo "Rollback command (manual; not run automatically during development): $repo/deploy/freebsd/rollback.sh $stamp"
