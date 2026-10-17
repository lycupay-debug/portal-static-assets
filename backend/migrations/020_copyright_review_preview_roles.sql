BEGIN;
ALTER TABLE portal.portal_users ADD COLUMN IF NOT EXISTS is_copyright_reviewer BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS portal_users_copyright_reviewer_idx ON portal.portal_users(user_id) WHERE is_copyright_reviewer=TRUE;
INSERT INTO portal.schema_migrations(version, name) VALUES (20, 'copyright_review_preview_roles') ON CONFLICT (version) DO NOTHING;
COMMIT;
