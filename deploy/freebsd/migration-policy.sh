#!/bin/sh

# Execute each migration atomically. Files that own their transaction retain
# that control; historical files without BEGIN/COMMIT use psql's wrapper.
personal_portal_apply_migration() {
  psql_bin=$1
  db_name=$2
  migration=$3
  if grep -Eq '^[[:space:]]*BEGIN[[:space:]]*;' "$migration"; then
    "$psql_bin" -v ON_ERROR_STOP=1 -d "$db_name" -f "$migration"
  else
    "$psql_bin" --single-transaction -v ON_ERROR_STOP=1 -d "$db_name" -f "$migration"
  fi
}
