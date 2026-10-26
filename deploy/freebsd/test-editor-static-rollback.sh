#!/bin/sh
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
tmp=$(mktemp -d /var/tmp/editor-static-rollback-test.XXXXXX)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
www="$tmp/www"
backups="$tmp/backups"
caddy="$tmp/Caddyfile"
stamp=20260101T010203Z
release="$www/releases/$stamp-independent-editor"
previous="$www/releases/previous"
mkdir -p "$release/editor" "$previous" "$backups"
ln -s "$release" "$www/current"
printf 'current editor config\n' > "$caddy"
printf 'previous portal config\n' > "$backups/Caddyfile.pre-editor-$stamp"
printf '%s\n' "$previous" > "$backups/frontend-current-pre-editor-$stamp"

PERSONAL_PORTAL_EDITOR_ROLLBACK_TEST_MODE=1 \
PERSONAL_PORTAL_WWW_BASE="$www" \
PERSONAL_PORTAL_BACKUP_DIR="$backups" \
PERSONAL_PORTAL_CADDYFILE="$caddy" \
sh "$repo/deploy/freebsd/rollback-editor-static.sh" "$stamp"

actual=$(readlink "$www/current")
[ "$actual" = "$previous" ]
grep -q 'previous portal config' "$caddy"

if PERSONAL_PORTAL_EDITOR_ROLLBACK_TEST_MODE=1 \
  PERSONAL_PORTAL_WWW_BASE="$www" \
  PERSONAL_PORTAL_BACKUP_DIR="$backups" \
  PERSONAL_PORTAL_CADDYFILE="$caddy" \
  sh "$repo/deploy/freebsd/rollback-editor-static.sh" "$stamp" >/dev/null 2>&1; then
  echo 'FAIL: stale rollback should be rejected.' >&2
  exit 1
fi

echo 'PASS: static editor rollback restores the previous release/config and rejects stale targets.'
