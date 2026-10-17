#!/bin/sh
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
sh "$repo/deploy/freebsd/test-deploy-policy.sh"
sh "$repo/deploy/freebsd/test-migration-transaction.sh"
sh "$repo/deploy/freebsd/test-caddy-security-headers.sh"
tmp=$(mktemp -d /tmp/personal-portal-rollback-test.XXXXXX)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

bin_dir="$tmp/bin"
backup_dir="$tmp/backups"
www_base="$tmp/www"
caddy_dir="$tmp/etc/caddy"
mkdir -p "$bin_dir" "$backup_dir" "$www_base/releases/previous" "$www_base/releases/current" "$caddy_dir"

printf 'new-go-binary\n' > "$bin_dir/api"
printf 'new-event-core\n' > "$bin_dir/event-core"
printf 'new-caddy-config\n' > "$caddy_dir/Caddyfile"
printf 'previous frontend\n' > "$www_base/releases/previous/index.html"
printf 'current frontend\n' > "$www_base/releases/current/index.html"
ln -s "$www_base/releases/current" "$www_base/current"

stamp="rollback-test"
printf 'old-go-binary\n' > "$bin_dir/api.pre-go-cutover-$stamp"
printf 'old-event-core\n' > "$bin_dir/event-core.pre-go-cutover-$stamp"
printf 'old-caddy-config\n' > "$caddy_dir/Caddyfile.pre-go-cutover-$stamp"
printf '%s\n' "$www_base/releases/previous" > "$backup_dir/frontend-current-$stamp"

PERSONAL_PORTAL_ROLLBACK_TEST_MODE=1 \
PERSONAL_PORTAL_BIN_DIR="$bin_dir" \
PERSONAL_PORTAL_CADDYFILE="$caddy_dir/Caddyfile" \
PERSONAL_PORTAL_BACKUP_DIR="$backup_dir" \
PERSONAL_PORTAL_WWW_BASE="$www_base" \
  "$repo/deploy/freebsd/rollback.sh" "$stamp"

grep -qx 'old-go-binary' "$bin_dir/api"
grep -qx 'old-event-core' "$bin_dir/event-core"
grep -qx 'old-caddy-config' "$caddy_dir/Caddyfile"
[ "$(readlink "$www_base/current")" = "$www_base/releases/previous" ]
grep -qx 'previous frontend' "$www_base/current/index.html"

echo "PASS: rollback restores API/C binaries, Caddy configuration, and frontend release in an isolated sandbox."

# Verify that a full backup archive can replace loose rollback copies after cleanup.
archive_bin="$tmp/archive-bin"
archive_backup="$tmp/archive-backups"
archive_www="$tmp/archive-www"
archive_caddy="$tmp/archive-etc/caddy"
archive_source="$tmp/archive-source"
mkdir -p "$archive_bin" "$archive_backup" "$archive_www/releases/current" "$archive_caddy" "$archive_source/rollback/previous-frontend"
printf 'new-go-binary\n' > "$archive_bin/api"
printf 'new-event-core\n' > "$archive_bin/event-core"
printf 'new-caddy-config\n' > "$archive_caddy/Caddyfile"
printf 'current frontend\n' > "$archive_www/releases/current/index.html"
ln -s "$archive_www/releases/current" "$archive_www/current"
archive_stamp="archive-test"
printf 'old-go-binary\n' > "$archive_source/rollback/api.pre-go-cutover-$archive_stamp"
printf 'old-event-core\n' > "$archive_source/rollback/event-core.pre-go-cutover-$archive_stamp"
printf 'old-caddy-config\n' > "$archive_source/rollback/Caddyfile.pre-go-cutover-$archive_stamp"
printf '%s\n' "$archive_www/releases/previous" > "$archive_source/rollback/frontend-current-$archive_stamp"
printf 'previous frontend\n' > "$archive_source/rollback/previous-frontend/index.html"
tar -czf "$archive_backup/final-backup-$archive_stamp.tar.gz" -C "$archive_source" .

PERSONAL_PORTAL_ROLLBACK_TEST_MODE=1 \
PERSONAL_PORTAL_FULL_BACKUP="$archive_backup/final-backup-$archive_stamp.tar.gz" \
PERSONAL_PORTAL_BIN_DIR="$archive_bin" \
PERSONAL_PORTAL_CADDYFILE="$archive_caddy/Caddyfile" \
PERSONAL_PORTAL_BACKUP_DIR="$archive_backup" \
PERSONAL_PORTAL_WWW_BASE="$archive_www" \
  "$repo/deploy/freebsd/rollback.sh" "$archive_stamp"

grep -qx 'old-go-binary' "$archive_bin/api"
grep -qx 'old-event-core' "$archive_bin/event-core"
grep -qx 'old-caddy-config' "$archive_caddy/Caddyfile"
[ "$(readlink "$archive_www/current")" = "$archive_www/releases/previous" ]
grep -qx 'previous frontend' "$archive_www/current/index.html"
echo "PASS: rollback can restore from the full backup archive after loose rollback copies and the previous release are cleaned."
