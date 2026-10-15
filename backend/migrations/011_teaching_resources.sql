BEGIN;
CREATE TABLE IF NOT EXISTS portal.teaching_resources (
  resource_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id UUID NOT NULL REFERENCES portal.portal_users(user_id) ON DELETE RESTRICT,
  title TEXT NOT NULL CHECK (char_length(btrim(title)) BETWEEN 1 AND 200),
  description TEXT NOT NULL DEFAULT '' CHECK (char_length(description) <= 2000),
  original_filename TEXT NOT NULL CHECK (char_length(original_filename) BETWEEN 1 AND 255),
  stored_filename TEXT NOT NULL UNIQUE,
  mime_type TEXT NOT NULL,
  file_size BIGINT NOT NULL CHECK (file_size > 0),
  deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS teaching_resources_owner_updated_idx ON portal.teaching_resources(owner_id, updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS teaching_resources_trash_idx ON portal.teaching_resources(owner_id, deleted_at DESC) WHERE deleted_at IS NOT NULL;
ALTER TABLE portal.teaching_resources OWNER TO personal_portal_app;
REVOKE ALL ON TABLE portal.teaching_resources FROM PUBLIC;
INSERT INTO portal.schema_migrations(version, name) VALUES (11, 'teaching_resources') ON CONFLICT (version) DO NOTHING;
COMMIT;
