#!/bin/sh
# Read-only PostgreSQL diagnostics for the production portal.
# Always use the application's protected database URL; never rely on psql's
# implicit OS-user/database defaults (which may select role/database "root").
set -eu

DB_URL_FILE="${PERSONAL_PORTAL_DATABASE_URL_FILE:-/usr/local/etc/personal-portal/database-url}"
if [ ! -r "$DB_URL_FILE" ]; then
  echo "Database URL file is missing or unreadable: $DB_URL_FILE" >&2
  exit 1
fi
DB_URL=$(cat "$DB_URL_FILE")
if [ -z "$DB_URL" ]; then
  echo "Database URL file is empty: $DB_URL_FILE" >&2
  exit 1
fi

exec psql "$DB_URL" -X -v ON_ERROR_STOP=1 <<'SQL'
\pset pager off
\echo '=== Connection and server ==='
SELECT current_database() AS database_name,
       current_user AS database_role,
       current_setting('server_version') AS server_version;

\echo '=== Portal tables and actual columns ==='
SELECT table_name, column_name, data_type
FROM information_schema.columns
WHERE table_schema = 'portal'
  AND table_name IN ('portal_content', 'teaching_resources')
ORDER BY table_name, ordinal_position;

\echo '=== Portal index health and usage ==='
SELECT s.schemaname,
       s.relname AS table_name,
       s.indexrelname AS index_name,
       s.idx_scan,
       s.idx_tup_read,
       s.idx_tup_fetch,
       i.indisvalid AS index_valid,
       i.indisready AS index_ready
FROM pg_stat_user_indexes AS s
JOIN pg_index AS i ON i.indexrelid = s.indexrelid
WHERE s.schemaname = 'portal'
ORDER BY s.relname, s.indexrelname;

\echo '=== Search-related index definitions ==='
SELECT p.tablename AS table_name, p.indexname AS index_name, p.indexdef
FROM pg_indexes AS p
WHERE p.schemaname = 'portal'
  AND (p.indexname LIKE '%search%' OR p.indexname LIKE '%trgm%'
       OR p.indexname LIKE '%fuzzy%' OR p.indexname LIKE '%compact%')
ORDER BY p.tablename, p.indexname;
SQL
