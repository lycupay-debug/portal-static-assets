-- Separate runtime DML permissions from ownership/DDL privileges.
-- This migration must be executed by a database administrator.
BEGIN;

DO $role$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'personal_portal_owner') THEN
    CREATE ROLE personal_portal_owner NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION;
  END IF;
END
$role$;

ALTER SCHEMA portal OWNER TO personal_portal_owner;

DO $objects$
DECLARE obj record;
BEGIN
  FOR obj IN
    SELECT c.relkind, n.nspname, c.relname
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'portal'
      AND c.relkind IN ('r', 'p', 'v', 'm', 'f', 'S')
    ORDER BY CASE c.relkind WHEN 'S' THEN 2 ELSE 1 END, c.relname
  LOOP
    IF obj.relkind = 'S' THEN
      EXECUTE format('ALTER SEQUENCE %I.%I OWNER TO personal_portal_owner', obj.nspname, obj.relname);
    ELSIF obj.relkind IN ('r', 'p') THEN
      EXECUTE format('ALTER TABLE %I.%I OWNER TO personal_portal_owner', obj.nspname, obj.relname);
    ELSIF obj.relkind = 'v' THEN
      EXECUTE format('ALTER VIEW %I.%I OWNER TO personal_portal_owner', obj.nspname, obj.relname);
    ELSIF obj.relkind = 'm' THEN
      EXECUTE format('ALTER MATERIALIZED VIEW %I.%I OWNER TO personal_portal_owner', obj.nspname, obj.relname);
    ELSIF obj.relkind = 'f' THEN
      EXECUTE format('ALTER FOREIGN TABLE %I.%I OWNER TO personal_portal_owner', obj.nspname, obj.relname);
    END IF;
  END LOOP;

  FOR obj IN
    SELECT p.oid::regprocedure AS signature
    FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
    WHERE n.nspname = 'portal'
  LOOP
    EXECUTE format('ALTER FUNCTION %s OWNER TO personal_portal_owner', obj.signature);
  END LOOP;
END
$objects$;

-- Runtime gets only schema usage and normal DML, not object ownership, DDL,
-- TRUNCATE, REFERENCES, or TRIGGER privileges.
REVOKE ALL ON SCHEMA portal FROM PUBLIC;
REVOKE CREATE ON SCHEMA portal FROM personal_portal_app;
GRANT USAGE ON SCHEMA portal TO personal_portal_app;
REVOKE ALL ON ALL TABLES IN SCHEMA portal FROM personal_portal_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA portal TO personal_portal_app;
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA portal TO personal_portal_app;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA portal TO personal_portal_app;

-- The runtime may read and advance the migration ledger only if a future
-- deployment accidentally uses the app identity for status checks.
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE portal.schema_migrations TO personal_portal_app;

-- New objects created by the dedicated owner automatically receive the same
-- runtime privileges. The deployment runner also normalizes ownership after
-- every pending migration so legacy ALTER OWNER statements cannot re-open the gap.
ALTER DEFAULT PRIVILEGES FOR ROLE personal_portal_owner IN SCHEMA portal
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO personal_portal_app;
ALTER DEFAULT PRIVILEGES FOR ROLE personal_portal_owner IN SCHEMA portal
  GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO personal_portal_app;
ALTER DEFAULT PRIVILEGES FOR ROLE personal_portal_owner IN SCHEMA portal
  GRANT EXECUTE ON FUNCTIONS TO personal_portal_app;

INSERT INTO portal.schema_migrations(version, name)
VALUES (30, 'database_owner_runtime_separation')
ON CONFLICT (version) DO NOTHING;

COMMIT;
