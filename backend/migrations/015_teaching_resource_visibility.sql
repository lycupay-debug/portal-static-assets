BEGIN;
ALTER TABLE portal.teaching_resources ADD COLUMN IF NOT EXISTS is_public BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS teaching_resources_public_updated_idx ON portal.teaching_resources(updated_at DESC) WHERE is_public = TRUE AND deleted_at IS NULL;
INSERT INTO portal.schema_migrations(version, name) VALUES (15, 'teaching_resource_visibility') ON CONFLICT (version) DO NOTHING;
COMMIT;
