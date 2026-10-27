ALTER SCHEMA portal OWNER TO personal_portal_owner;
DO $normalize$
DECLARE obj record;
BEGIN
  FOR obj IN SELECT c.relkind,n.nspname,c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='portal' AND c.relkind IN ('r','p','v','m','f','S') LOOP
    IF obj.relkind='S' THEN EXECUTE format('ALTER SEQUENCE %I.%I OWNER TO personal_portal_owner',obj.nspname,obj.relname);
    ELSIF obj.relkind IN ('r','p') THEN EXECUTE format('ALTER TABLE %I.%I OWNER TO personal_portal_owner',obj.nspname,obj.relname);
    ELSIF obj.relkind='v' THEN EXECUTE format('ALTER VIEW %I.%I OWNER TO personal_portal_owner',obj.nspname,obj.relname);
    ELSIF obj.relkind='m' THEN EXECUTE format('ALTER MATERIALIZED VIEW %I.%I OWNER TO personal_portal_owner',obj.nspname,obj.relname);
    ELSIF obj.relkind='f' THEN EXECUTE format('ALTER FOREIGN TABLE %I.%I OWNER TO personal_portal_owner',obj.nspname,obj.relname); END IF;
  END LOOP;
  FOR obj IN SELECT p.oid::regprocedure AS signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='portal' LOOP
    EXECUTE format('ALTER FUNCTION %s OWNER TO personal_portal_owner',obj.signature);
  END LOOP;
END
$normalize$;
REVOKE CREATE ON SCHEMA portal FROM personal_portal_app;
GRANT USAGE ON SCHEMA portal TO personal_portal_app;
REVOKE ALL ON ALL TABLES IN SCHEMA portal FROM personal_portal_app;
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA portal TO personal_portal_app;
GRANT USAGE,SELECT,UPDATE ON ALL SEQUENCES IN SCHEMA portal TO personal_portal_app;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA portal TO personal_portal_app;
GRANT SELECT,INSERT,UPDATE,DELETE ON TABLE portal.schema_migrations TO personal_portal_app;
