#!/bin/sh

# Execute each migration atomically. Production migration SQL runs as the local
# PostgreSQL administrator via stdin so postgres need not traverse the
# root-owned repository tree. Tests can omit the privileged mode.
personal_portal_apply_migration() {
  psql_bin=$1
  db_name=$2
  migration=$3
  if [ "${PERSONAL_PORTAL_MIGRATION_AS_POSTGRES:-0}" = "1" ]; then
    if grep -Eq '^[[:space:]]*BEGIN[[:space:]]*;' "$migration"; then
      su -m postgres -c "env -u PGHOST -u PGPORT -u PGUSER -u PGPASSWORD -u PGSSLMODE '$psql_bin' -v ON_ERROR_STOP=1 -d '$db_name' -f -" < "$migration"
    else
      su -m postgres -c "env -u PGHOST -u PGPORT -u PGUSER -u PGPASSWORD -u PGSSLMODE '$psql_bin' --single-transaction -v ON_ERROR_STOP=1 -d '$db_name' -f -" < "$migration"
    fi
  elif grep -Eq '^[[:space:]]*BEGIN[[:space:]]*;' "$migration"; then
    "$psql_bin" -v ON_ERROR_STOP=1 -d "$db_name" -f "$migration"
  else
    "$psql_bin" --single-transaction -v ON_ERROR_STOP=1 -d "$db_name" -f "$migration"
  fi
}
