#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root." >&2
  exit 1
fi

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
migration_dir="$repo/backend/migrations"
psql_bin=$(command -v psql)
python_bin=$(command -v python3.14)
db_name="${PERSONAL_PORTAL_DB_NAME:-personal_portal}"
db_url_file="${PERSONAL_PORTAL_DATABASE_URL_FILE:-/usr/local/etc/personal-portal/database-url}"
backup="${PERSONAL_PORTAL_MIGRATION_BACKUP:-}"

if [ -z "$backup" ] || [ ! -s "$backup" ]; then
  echo "Set PERSONAL_PORTAL_MIGRATION_BACKUP to a verified full database dump before applying migrations." >&2
  exit 1
fi
if [ ! -r "$db_url_file" ]; then
  echo "Database URL file is missing or unreadable: $db_url_file" >&2
  exit 1
fi

# Parse the protected database URL into libpq environment variables. The
# password is never echoed and is not passed as a command-line argument.
eval "$("$python_bin" - "$db_url_file" <<'PY'
import shlex
import sys
from pathlib import Path
from urllib.parse import parse_qs, unquote, urlparse

u = urlparse(Path(sys.argv[1]).read_text().strip())
values = {
    "PGHOST": u.hostname or "127.0.0.1",
    "PGPORT": str(u.port or 5432),
    "PGUSER": unquote(u.username or ""),
    "PGPASSWORD": unquote(u.password or ""),
    "PGSSLMODE": parse_qs(u.query).get("sslmode", ["prefer"])[0],
}
for key, value in values.items():
    print("export " + key + "=" + shlex.quote(value))
PY
)"
export PGDATABASE="$db_name"

for migration in "$migration_dir"/[0-9][0-9][0-9]_*.sql; do
  version=$(basename "$migration" | cut -d_ -f1)
  if [ "$version" = "001" ]; then
    exists=$("$psql_bin" -At -v ON_ERROR_STOP=1 -d "$db_name" -c "SELECT to_regclass('portal.schema_migrations') IS NOT NULL" 2>/dev/null || true)
    if [ "$exists" = "t" ]; then
      applied=$("$psql_bin" -At -v ON_ERROR_STOP=1 -d "$db_name" -c "SELECT EXISTS(SELECT 1 FROM portal.schema_migrations WHERE version=1)")
      if [ "$applied" = "t" ]; then
        echo "SKIP migration 001 (already applied)"
        continue
      fi
    fi
  else
    exists=$("$psql_bin" -At -v ON_ERROR_STOP=1 -d "$db_name" -c "SELECT to_regclass('portal.schema_migrations') IS NOT NULL")
    if [ "$exists" != "t" ]; then
      echo "Migration ledger missing before $migration; applying foundation first is required." >&2
      exit 1
    fi
    applied=$("$psql_bin" -At -v ON_ERROR_STOP=1 -d "$db_name" -c "SELECT EXISTS(SELECT 1 FROM portal.schema_migrations WHERE version=$version)")
    if [ "$applied" = "t" ]; then
      echo "SKIP migration $version (already applied)"
      continue
    fi
  fi
  echo "APPLY migration $version: $(basename "$migration")"
  "$psql_bin" -v ON_ERROR_STOP=1 -d "$db_name" -f "$migration"
done

"$psql_bin" -v ON_ERROR_STOP=1 -d "$db_name" -c 'SELECT version,name,applied_at FROM portal.schema_migrations ORDER BY version;'
