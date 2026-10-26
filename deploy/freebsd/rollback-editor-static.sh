#!/bin/sh
set -eu

stamp="${1:-}"
case "$stamp" in
  ????????T??????Z) ;;
  *) echo "Usage: $0 YYYYMMDDTHHMMSSZ" >&2; exit 2 ;;
esac
case "$stamp" in
  *[!0-9TZ]*) echo "Invalid deployment stamp." >&2; exit 2 ;;
esac

test_mode="${PERSONAL_PORTAL_EDITOR_ROLLBACK_TEST_MODE:-0}"
www_base="${PERSONAL_PORTAL_WWW_BASE:-/usr/local/www/personal-portal}"
backup_dir="${PERSONAL_PORTAL_BACKUP_DIR:-/var/backups/personal-portal}"
caddy_file="${PERSONAL_PORTAL_CADDYFILE:-/usr/local/etc/caddy/Caddyfile}"
release="$www_base/releases/$stamp-independent-editor"
previous_file="$backup_dir/frontend-current-pre-editor-$stamp"
caddy_backup="$backup_dir/Caddyfile.pre-editor-$stamp"
forward_config_backup="$backup_dir/Caddyfile.pre-static-rollback-$stamp"
forward_link_backup="$backup_dir/frontend-current-pre-static-rollback-$stamp"

if [ "$test_mode" != "1" ] && [ "$(id -u)" -ne 0 ]; then
  echo "Run as root." >&2
  exit 1
fi
for required in "$release" "$previous_file" "$caddy_backup" "$www_base/current"; do
  if [ ! -e "$required" ]; then
    echo "Missing static rollback artifact: $required" >&2
    exit 1
  fi
done
if [ ! -L "$www_base/current" ]; then
  echo "The current release is not a symlink; refusing to change it." >&2
  exit 1
fi
active_release=$(readlink "$www_base/current")
if [ "$active_release" != "$release" ]; then
  echo "Active release is '$active_release', not '$release'; refusing stale rollback." >&2
  exit 1
fi
previous_release=$(cat "$previous_file")
if [ ! -d "$previous_release" ]; then
  echo "Previous release is missing: $previous_release" >&2
  exit 1
fi

cp -p "$caddy_file" "$forward_config_backup"
chmod 0600 "$forward_config_backup"
readlink "$www_base/current" > "$forward_link_backup"
chmod 0600 "$forward_link_backup"

restore_failed_rollback() {
  rc=$?
  if [ "$rc" -ne 0 ]; then
    echo "Static rollback failed; restoring the editor release and its current Caddy config." >&2
    install -o root -g wheel -m 0644 "$forward_config_backup" "$caddy_file" 2>/dev/null || cp "$forward_config_backup" "$caddy_file"
    ln -s "$release" "$www_base/current.restore" 2>/dev/null || true
    if [ -L "$www_base/current.restore" ]; then mv -h -f "$www_base/current.restore" "$www_base/current"; fi
    if [ "$test_mode" != "1" ]; then service caddy reload || true; fi
  fi
  exit "$rc"
}
trap restore_failed_rollback EXIT

if [ "$test_mode" != "1" ]; then
  caddy validate --config "$caddy_backup"
fi
install -o root -g wheel -m 0644 "$caddy_backup" "$caddy_file" 2>/dev/null || cp "$caddy_backup" "$caddy_file"
if [ "$test_mode" != "1" ]; then caddy validate --config "$caddy_file"; fi
ln -s "$previous_release" "$www_base/current.rollback-editor"
mv -h -f "$www_base/current.rollback-editor" "$www_base/current"
if [ "$test_mode" != "1" ]; then
  service caddy reload
  curl -fsS --max-time 10 https://www.lycupaynotion.top/ -o /dev/null
fi

trap - EXIT
echo "Static editor rollback completed for deployment stamp $stamp."
echo "Previous release restored: $previous_release"
echo "Forward recovery config: $forward_config_backup"
echo "Forward recovery release pointer: $forward_link_backup"
