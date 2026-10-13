-- Personal Portal database foundation. Apply manually as the database owner.
CREATE SCHEMA IF NOT EXISTS portal AUTHORIZATION personal_portal_app;

CREATE TABLE IF NOT EXISTS portal.schema_migrations (
    version bigint PRIMARY KEY,
    name text NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
);

REVOKE ALL ON SCHEMA portal FROM PUBLIC;
REVOKE ALL ON TABLE portal.schema_migrations FROM PUBLIC;

INSERT INTO portal.schema_migrations (version, name)
VALUES (1, 'portal_foundation')
ON CONFLICT (version) DO NOTHING;
