#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root." >&2
  exit 1
fi

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
bin_dir="/usr/local/libexec/personal-portal"
www_base="/usr/local/www/personal-portal"
caddy_file="/usr/local/etc/caddy/Caddyfile"
backup_dir="/var/backups/personal-portal"
legacy_node_dir="${PERSONAL_PORTAL_LEGACY_NODE_DIR:-/opt/personal-portal-legacy-node}"
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

if [ ! -s "$legacy_node_dir/backend/auth-api/server.mjs" ] || [ ! -s "$legacy_node_dir/backend/content-api/handlers.mjs" ]; then
  echo "Legacy Node rollback archive is missing: $legacy_node_dir" >&2
  exit 1
fi

if [ -n "$(git -C "$repo" status --porcelain)" ]; then
  echo "Repository must be clean and committed before deployment." >&2
  exit 1
fi

sh "$repo/deploy/freebsd/test-caddy-security-headers.sh"

cd "$repo/frontend"
npm ci
python3.14 "$repo/scripts/compliance/scan_assets.py" --repo "$repo" --release "$www_base/current" --out "$work/compliance-preflight" --strict-licenses
npm run api:lint
npm run api:check
npm run type-check
npm test
npm run build

cd "$repo/backend"
"$go_bin" test -race ./...
"$go_bin" vet ./...
"$go_bin" build -trimpath -ldflags="-s -w" -o "$work/api" ./api

cd "$repo/event-core"
make clean all test

# Create a full PostgreSQL backup before applying migrations or replacing services.
dump_tmp="/tmp/personal-portal-pre-go-cutover-$stamp.dump"
su -m postgres -c "pg_dump -Fc -d personal_portal -f '$dump_tmp'"
install -o root -g wheel -m 0600 "$dump_tmp" "$backup_dir/pre-go-cutover-$stamp.dump"
rm -f "$dump_tmp"

cp "$bin_dir/api" "$bin_dir/api.pre-go-cutover-$stamp"
cp "$bin_dir/event-core" "$bin_dir/event-core.pre-go-cutover-$stamp"
cp "$caddy_file" "$caddy_file.pre-go-cutover-$stamp"
readlink "$www_base/current" > "$backup_dir/frontend-current-$stamp"
chmod 0600 "$backup_dir/frontend-current-$stamp"

# Forward-only additive migrations run after the full backup.
PERSONAL_PORTAL_MIGRATION_BACKUP="$backup_dir/pre-go-cutover-$stamp.dump" "$repo/deploy/freebsd/migrate.sh"

release="$www_base/releases/${stamp}-go-backend"
mkdir -p "$release"
cp -R "$repo/frontend/dist/." "$release/"
if [ ! -f "$release/favicon.ico" ] && [ -f "$www_base/current/favicon.ico" ]; then
  cp "$www_base/current/favicon.ico" "$release/favicon.ico"
fi

deployment_changed=1
if [ "${PERSONAL_PORTAL_TASK_EXECUTOR_DEPLOY:-0}" != "1" ] && [ -x /usr/local/etc/rc.d/personal_portal_task_executor ]; then
  service personal_portal_task_executor stop || true
fi
service personal_portal_api stop
service personal_portal_event_core stop
install -o root -g wheel -m 0555 "$work/api" "$bin_dir/api"
install -o root -g wheel -m 0555 "$repo/event-core/event-core" "$bin_dir/event-core"
install -o root -g wheel -m 0555 "$repo/deploy/freebsd/personal_portal_task_executor" /usr/local/etc/rc.d/personal_portal_task_executor
install -d -o personalportal -g personalportal -m 0700 /var/db/personal-portal/task-home
install -d -o personalportal -g personalportal -m 0700 /var/db/personal-portal/task-home/go/pkg/mod
if [ -d /root/go/pkg/mod ] && [ ! -d /var/db/personal-portal/task-home/go/pkg/mod/cache/download ]; then
  cp -Rp /root/go/pkg/mod/. /var/db/personal-portal/task-home/go/pkg/mod/
  chown -R personalportal:personalportal /var/db/personal-portal/task-home/go/pkg/mod
fi
install -d -o personalportal -g personalportal -m 0750 "$repo/frontend/dist"
install -d -o personalportal -g personalportal -m 0700 "$repo/frontend/node_modules/.tmp" "$repo/frontend/node_modules/.cache"
chown personalportal:personalportal "$repo/frontend/dist" "$repo/frontend/node_modules/.tmp" "$repo/frontend/node_modules/.cache"
chmod 0750 "$repo/frontend/dist"
chmod 0700 "$repo/frontend/node_modules/.tmp" "$repo/frontend/node_modules/.cache"
sysrc personal_portal_task_executor_enable=YES >/dev/null
service personal_portal_event_core start
service personal_portal_api start
if [ "${PERSONAL_PORTAL_TASK_EXECUTOR_DEPLOY:-0}" != "1" ]; then
  service personal_portal_task_executor start
fi

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

# The privileged executor must report a fresh heartbeat before traffic switches.
attempt=0
while [ "$attempt" -lt 30 ]; do
  heartbeat=$(su -m postgres -c "psql -d personal_portal -X -At -v ON_ERROR_STOP=1 -c \"SELECT 1 FROM portal.command_executor_heartbeat WHERE singleton=TRUE AND status='ready' AND last_seen_at > now()-interval '15 seconds'\"" 2>/dev/null || true)
  if [ "$heartbeat" = "1" ]; then break; fi
  attempt=$((attempt + 1))
  sleep 1
done
if [ "$heartbeat" != "1" ]; then
  echo "Privileged command executor did not become ready; inspect /var/log/personal-portal/task-executor.log" >&2
  exit 1
fi

install -o root -g wheel -m 0644 "$repo/deploy/freebsd/Caddyfile" "$caddy_file"
caddy validate --config "$caddy_file"
service caddy reload
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
