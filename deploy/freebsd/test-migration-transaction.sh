#!/bin/sh
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
. "$repo/deploy/freebsd/migration-policy.sh"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/portal-migration-policy.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM

cat > "$tmp/fake-psql" <<'PSQL'
#!/bin/sh
printf '%s\n' "$*" >> "$MIGRATION_CALL_LOG"
PSQL
chmod +x "$tmp/fake-psql"
export MIGRATION_CALL_LOG="$tmp/calls.log"
: > "$MIGRATION_CALL_LOG"
printf 'CREATE TABLE legacy_migration_test(id integer);\n' > "$tmp/legacy.sql"
printf 'BEGIN;\nCREATE TABLE explicit_migration_test(id integer);\nCOMMIT;\n' > "$tmp/explicit.sql"

personal_portal_apply_migration "$tmp/fake-psql" portal_test "$tmp/legacy.sql"
personal_portal_apply_migration "$tmp/fake-psql" portal_test "$tmp/explicit.sql"

first=$(sed -n '1p' "$MIGRATION_CALL_LOG")
second=$(sed -n '2p' "$MIGRATION_CALL_LOG")
case "$first" in
  *--single-transaction*"-d portal_test"*"$tmp/legacy.sql") ;;
  *) echo "FAIL: legacy migration was not wrapped atomically: $first" >&2; exit 1 ;;
esac
case "$second" in
  *"-d portal_test"*"$tmp/explicit.sql") ;;
  *) echo "FAIL: explicit migration was not invoked: $second" >&2; exit 1 ;;
esac
case "$second" in
  *--single-transaction*) echo "FAIL: explicit transaction was double-wrapped: $second" >&2; exit 1 ;;
esac

echo 'PASS: legacy migrations use a single transaction; explicit transaction migrations are not double-wrapped.'
