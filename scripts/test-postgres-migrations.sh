#!/bin/sh
# Replays every numbered migration into a disposable local PostgreSQL database
# and verifies the runtime role boundary. This is a CI/test-only script; it
# refuses remote hosts and requires an explicit opt-in to avoid production use.
set -eu

if [ "${PORTAL_MIGRATION_TEST:-}" != "1" ]; then
  echo "Set PORTAL_MIGRATION_TEST=1 to confirm this is a disposable test run." >&2
  exit 2
fi

pg_host=${PGHOST:-}
pg_port=${PGPORT:-5432}
pg_user=${PGUSER:-postgres}
psql_bin=${PSQL_BIN:-psql}
test_db=portal_migration_test
app_password=portal-ci-only-password

case "$pg_host" in
  127.0.0.1|localhost|/var/tmp/*) ;;
  *)
    echo "Refusing non-local PostgreSQL host: ${pg_host:-<empty>}" >&2
    exit 2
    ;;
esac
if [ "$pg_user" != "postgres" ]; then
  echo "This test requires the disposable cluster administrator PGUSER=postgres." >&2
  exit 2
fi
if ! command -v "$psql_bin" >/dev/null 2>&1; then
  echo "psql client not found: $psql_bin" >&2
  exit 1
fi

repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
migration_dir="$repo/backend/migrations"
psql_admin() {
  "$psql_bin" -X -v ON_ERROR_STOP=1 -h "$pg_host" -p "$pg_port" -U "$pg_user" "$@"
}

db_exists=$(psql_admin -d postgres -Atc "SELECT 1 FROM pg_database WHERE datname='$test_db'")
role_exists=$(psql_admin -d postgres -Atc "SELECT 1 FROM pg_roles WHERE rolname='personal_portal_app'")
if [ "$db_exists" = "1" ] || [ "$role_exists" = "1" ]; then
  echo "Refusing to overwrite an existing $test_db database or personal_portal_app role." >&2
  exit 2
fi

psql_admin -d postgres -c "CREATE ROLE personal_portal_app LOGIN PASSWORD '$app_password' NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION"
psql_admin -d postgres -c "CREATE DATABASE $test_db OWNER postgres"

for migration in "$migration_dir"/[0-9][0-9][0-9]_*.sql; do
  version=$(basename "$migration" | cut -d_ -f1)
  echo "APPLY $version $(basename "$migration")"
  if [ "$version" = "031" ]; then
    psql_admin -d "$test_db" -c "INSERT INTO portal.event_inbox(event_id,event_type,payload) VALUES('550e8400-e29b-41d4-a716-446655440031','portal.command.execute','{}'::jsonb)"
  fi
  if grep -Eq '^[[:space:]]*BEGIN[[:space:]]*;' "$migration"; then
    psql_admin -d "$test_db" -f "$migration"
  else
    psql_admin --single-transaction -d "$test_db" -f "$migration"
  fi
done

# Match the production migration runner's final owner/privilege normalization.
psql_admin -d "$test_db" -f "$repo/deploy/freebsd/normalize-db-ownership.sql"

ledger_count=$(psql_admin -d "$test_db" -Atc "SELECT count(*) FROM portal.schema_migrations")
ledger_range=$(psql_admin -d "$test_db" -Atc "SELECT min(version)||':'||max(version) FROM portal.schema_migrations")
if [ "$ledger_count" != "31" ] || [ "$ledger_range" != "1:31" ]; then
  echo "Unexpected migration ledger: count=$ledger_count range=$ledger_range" >&2
  exit 1
fi

retired_tables=$(psql_admin -d "$test_db" -Atc "SELECT count(*) FROM unnest(ARRAY['portal.command_tasks','portal.command_executor_heartbeat','portal.system_execution_policy']) AS retired(object_name) WHERE to_regclass(object_name) IS NOT NULL")
retired_permission=$(psql_admin -d "$test_db" -Atc "SELECT count(*) FROM portal.permissions WHERE permission_key='portal.system_ops.manage'")
read_permission=$(psql_admin -d "$test_db" -Atc "SELECT count(*) FROM portal.permissions WHERE permission_key='portal.system_ops.read'")
retired_events=$(psql_admin -d "$test_db" -Atc "SELECT count(*) FROM portal.event_inbox WHERE event_type='portal.command.execute'")
if [ "$retired_tables" != "0" ] || [ "$retired_permission" != "0" ] || [ "$read_permission" != "1" ] || [ "$retired_events" != "0" ]; then
  echo "Command executor retirement failed: tables=$retired_tables manage_permission=$retired_permission read_permission=$read_permission stale_events=$retired_events" >&2
  exit 1
fi

non_owner_relations=$(psql_admin -d "$test_db" -Atc "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='portal' AND c.relkind IN ('r','p','v','m','f','S','i','I') AND c.relowner<>(SELECT oid FROM pg_roles WHERE rolname='personal_portal_owner')")
non_owner_functions=$(psql_admin -d "$test_db" -Atc "SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='portal' AND p.proowner<>(SELECT oid FROM pg_roles WHERE rolname='personal_portal_owner')")
if [ "$non_owner_relations" != "0" ] || [ "$non_owner_functions" != "0" ]; then
  echo "Database owner separation failed: relations=$non_owner_relations functions=$non_owner_functions" >&2
  exit 1
fi

role_flags=$(psql_admin -d "$test_db" -Atc "SELECT (NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole AND rolcanlogin)::int FROM pg_roles WHERE rolname='personal_portal_app'")
owner_flags=$(psql_admin -d "$test_db" -Atc "SELECT (NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole AND NOT rolcanlogin)::int FROM pg_roles WHERE rolname='personal_portal_owner'")
owner_membership=$(psql_admin -d "$test_db" -Atc "SELECT count(*) FROM pg_auth_members WHERE member=(SELECT oid FROM pg_roles WHERE rolname='personal_portal_app') AND roleid=(SELECT oid FROM pg_roles WHERE rolname='personal_portal_owner')")
if [ "$role_flags" != "1" ] || [ "$owner_flags" != "1" ] || [ "$owner_membership" != "0" ]; then
  echo "Unexpected owner/runtime role attributes or role membership." >&2
  exit 1
fi

runtime_psql() {
  PGPASSWORD="$app_password" "$psql_bin" -X -v ON_ERROR_STOP=1 -h "$pg_host" -p "$pg_port" -U personal_portal_app -d "$test_db" "$@"
}

runtime_psql -c "INSERT INTO portal.portal_users(email,display_name,password_hash) VALUES('phase2-migration-test@example.invalid','Migration Test','test-only')"
default_role_count=$(runtime_psql -Atc "SELECT count(*) FROM portal.portal_users u JOIN portal.portal_user_roles ur USING(user_id) WHERE u.email='phase2-migration-test@example.invalid' AND ur.role_key='user'")
ledger_visible=$(runtime_psql -Atc "SELECT count(*) FROM portal.schema_migrations")
if [ "$default_role_count" != "1" ] || [ "$ledger_visible" != "31" ]; then
  echo "Runtime DML/default-role check failed: default_role=$default_role_count ledger_rows=$ledger_visible" >&2
  exit 1
fi

runtime_create=$(psql_admin -d "$test_db" -Atc "SELECT has_schema_privilege('personal_portal_app','portal','CREATE')::int")
runtime_truncate=$(psql_admin -d "$test_db" -Atc "SELECT has_table_privilege('personal_portal_app','portal.portal_users','TRUNCATE')::int")
if [ "$runtime_create" != "0" ] || [ "$runtime_truncate" != "0" ]; then
  echo "Runtime role has excessive privileges: CREATE=$runtime_create TRUNCATE=$runtime_truncate" >&2
  exit 1
fi
if runtime_psql -c "CREATE TABLE portal.phase2_should_not_exist(id integer)" >/dev/null 2>&1; then
  echo "Runtime role unexpectedly created a schema object." >&2
  exit 1
fi
if runtime_psql -c "TRUNCATE portal.portal_users" >/dev/null 2>&1; then
  echo "Runtime role unexpectedly truncated portal users." >&2
  exit 1
fi

echo "PostgreSQL migration, executor-retirement, and runtime-role checks passed: $ledger_count migrations ($ledger_range), retired tables and queued events absent, read-only system-ops permission retained, $non_owner_relations non-owner relations, $non_owner_functions non-owner functions."
