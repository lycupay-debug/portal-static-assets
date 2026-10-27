#!/bin/sh
set -eu
umask 077

repo=${1:-$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)}
go_bin=${2:-$(command -v go || true)}
if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root so the isolated test database can be created and migrated as PostgreSQL administrator." >&2
  exit 1
fi
if [ -z "$go_bin" ] || [ ! -x "$go_bin" ]; then
  echo "Go toolchain not found; pass its executable path as argument 2." >&2
  exit 1
fi

psql_bin=$(command -v psql)
createdb_bin=$(command -v createdb)
dropdb_bin=$(command -v dropdb)
db="personal_portal_test_$$"
url_file="/var/tmp/personal-portal-test-db-url-$$"
log_file="/var/tmp/personal-portal-test-db-$$.log"
created=0

cleanup() {
  status=$?
  rm -f "$url_file"
  if [ "$created" -eq 1 ]; then
    if ! "$dropdb_bin" -U postgres --if-exists "$db" >/dev/null 2>&1; then
      echo "WARNING: could not remove isolated test database $db." >&2
      status=1
    fi
  fi
  if [ "$status" -eq 0 ]; then
    rm -f "$log_file"
  else
    echo "PostgreSQL integration-test diagnostics preserved at $log_file" >&2
    if [ -f "$log_file" ]; then cat "$log_file" >&2; fi
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

if "$psql_bin" -X -U postgres -d postgres -At -c "SELECT 1 FROM pg_database WHERE datname='$db'" | grep -q '^1$'; then
  echo "Refusing to overwrite existing test database $db." >&2
  exit 1
fi

: > "$log_file"
echo "Creating isolated database: $db"
"$createdb_bin" -U postgres -O postgres "$db"
created=1

for migration in "$repo"/backend/migrations/[0-9][0-9][0-9]_*.sql; do
  version=$(basename "$migration" | cut -d_ -f1)
  echo "APPLY $version $(basename "$migration")" >> "$log_file"
  if grep -Eq '^[[:space:]]*BEGIN[[:space:]]*;' "$migration"; then
    if ! su -m postgres -c "env -u PGHOST -u PGPORT -u PGUSER -u PGPASSWORD -u PGSSLMODE '$psql_bin' -X -v ON_ERROR_STOP=1 -d '$db' -f -" < "$migration" >> "$log_file" 2>&1; then
      exit 1
    fi
  else
    if ! su -m postgres -c "env -u PGHOST -u PGPORT -u PGUSER -u PGPASSWORD -u PGSSLMODE '$psql_bin' -X --single-transaction -v ON_ERROR_STOP=1 -d '$db' -f -" < "$migration" >> "$log_file" 2>&1; then
      exit 1
    fi
  fi
done

if ! su -m postgres -c "env -u PGHOST -u PGPORT -u PGUSER -u PGPASSWORD -u PGSSLMODE '$psql_bin' -X -v ON_ERROR_STOP=1 -d '$db' -f -" < "$repo/deploy/freebsd/normalize-db-ownership.sql" >> "$log_file" 2>&1; then
  exit 1
fi

printf 'postgresql:///%s?host=/tmp&user=postgres\n' "$db" > "$url_file"
chmod 0600 "$url_file"
echo "Running Go tests with all PostgreSQL integration cases enabled."
(
  cd "$repo/backend"
  PERSONAL_PORTAL_TEST_DATABASE_URL_FILE="$url_file" "$go_bin" test -race -count=1 ./...
) >> "$log_file" 2>&1 || exit 1
cat "$log_file"
echo "PASS: migrations 001-030 and PostgreSQL-backed Go integration tests completed."
echo "The isolated database will be dropped automatically."
