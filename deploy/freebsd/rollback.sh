#!/bin/sh
set -eu

stamp="${1:-}"
if [ -z "$stamp" ]; then
  echo "Usage: $0 <deployment-stamp>" >&2
  exit 2
fi

test_mode="${PERSONAL_PORTAL_ROLLBACK_TEST_MODE:-0}"
bin_dir="${PERSONAL_PORTAL_BIN_DIR:-/usr/local/libexec/personal-portal}"
caddy_file="${PERSONAL_PORTAL_CADDYFILE:-/usr/local/etc/caddy/Caddyfile}"
backup_dir="${PERSONAL_PORTAL_BACKUP_DIR:-/var/backups/personal-portal}"
www_base="${PERSONAL_PORTAL_WWW_BASE:-/usr/local/www/personal-portal}"
legacy_node_dir="${PERSONAL_PORTAL_LEGACY_NODE_DIR:-/opt/personal-portal-legacy-node}"
api_backup="$bin_dir/api.pre-go-cutover-$stamp"
core_backup="$bin_dir/event-core.pre-go-cutover-$stamp"
caddy_backup="$caddy_file.pre-go-cutover-$stamp"
frontend_target_file="$backup_dir/frontend-current-$stamp"

for required in "$api_backup" "$core_backup" "$caddy_backup" "$frontend_target_file"; do
  if [ ! -e "$required" ]; then
    echo "Missing rollback artifact: $required" >&2
    exit 1
  fi
done

if [ "$test_mode" != "1" ]; then
  if [ ! -s "$legacy_node_dir/backend/auth-api/server.mjs" ] || [ ! -s "$legacy_node_dir/backend/content-api/handlers.mjs" ]; then
    echo "Legacy Node rollback archive is missing: $legacy_node_dir" >&2
    exit 1
  fi
fi

if [ "$test_mode" = "1" ]; then
  cp "$api_backup" "$bin_dir/api"
  cp "$core_backup" "$bin_dir/event-core"
  cp "$caddy_backup" "$caddy_file"
else
  if [ "$(id -u)" -ne 0 ]; then
    echo "Run as root." >&2
    exit 1
  fi
  service personal_portal_api stop || true
  service personal_portal_event_core stop || true
  install -o root -g wheel -m 0555 "$api_backup" "$bin_dir/api"
  install -o root -g wheel -m 0555 "$core_backup" "$bin_dir/event-core"
  install -o root -g wheel -m 0644 "$caddy_backup" "$caddy_file"
fi

previous_release=$(cat "$frontend_target_file")
ln -s "$previous_release" "$www_base/current.rollback"
if [ "$test_mode" = "1" ]; then
  rm -f "$www_base/current"
  mv "$www_base/current.rollback" "$www_base/current"
else
  mv -h -f "$www_base/current.rollback" "$www_base/current"
  service personal_portal_event_core start
  service personal_portal_api start
  caddy validate --config "$caddy_file"
  service caddy reload
  sysrc personal_portal_auth_api_dir="$legacy_node_dir/backend/auth-api" >/dev/null
  sysrc personal_portal_auth_api_enable=YES >/dev/null
  service personal_portal_auth_api start || true
  wait_for_url() {
    url="$1"
    attempt=0
    while [ "$attempt" -lt 30 ]; do
      if curl -fsS --max-time 3 "$url" >/dev/null 2>&1; then return 0; fi
      attempt=$((attempt + 1))
      sleep 1
    done
    echo "Rollback readiness timed out: $url" >&2
    return 1
  }
  wait_for_url http://127.0.0.1:8081/healthz
  wait_for_url http://127.0.0.1:8081/readyz
  wait_for_url https://www.lycupaynotion.top/
fi

echo "Rollback completed using deployment stamp $stamp."
