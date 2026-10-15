BEGIN;
ALTER TABLE portal.portal_content ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';
UPDATE portal.portal_content SET tags = '{}' WHERE tags IS NULL;
INSERT INTO portal.schema_migrations (version, name)
VALUES (9, 'portal_content_tags')
ON CONFLICT (version) DO NOTHING;
COMMIT;
