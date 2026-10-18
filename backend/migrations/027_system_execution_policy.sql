BEGIN;
CREATE TABLE IF NOT EXISTS portal.system_execution_policy (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton = TRUE),
    mode TEXT NOT NULL DEFAULT 'stable' CHECK (mode IN ('developer', 'stable')),
    changed_by TEXT NOT NULL DEFAULT 'system-initialization',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO portal.system_execution_policy(singleton, mode, changed_by)
VALUES (TRUE, 'stable', 'system-initialization')
ON CONFLICT (singleton) DO NOTHING;
ALTER TABLE portal.system_execution_policy OWNER TO personal_portal_app;
REVOKE ALL ON TABLE portal.system_execution_policy FROM PUBLIC;
INSERT INTO portal.schema_migrations(version, name)
VALUES (27, 'system_execution_policy')
ON CONFLICT (version) DO NOTHING;
COMMIT;
