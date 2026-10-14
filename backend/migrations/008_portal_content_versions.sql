BEGIN;

CREATE TABLE IF NOT EXISTS portal.portal_content_versions (
    version_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id UUID NOT NULL REFERENCES portal.portal_content(content_id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES portal.portal_users(user_id) ON DELETE RESTRICT,
    version_number INTEGER NOT NULL CHECK (version_number > 0),
    snapshot JSONB NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT portal_content_versions_number_unique UNIQUE (content_id, version_number)
);

CREATE INDEX IF NOT EXISTS portal_content_versions_author_created_idx
    ON portal.portal_content_versions (author_id, content_id, created_at DESC);

CREATE INDEX IF NOT EXISTS portal_content_trash_author_deleted_idx
    ON portal.portal_content (author_id, deleted_at DESC)
    WHERE deleted_at IS NOT NULL;

ALTER TABLE portal.portal_content_versions OWNER TO personal_portal_app;
REVOKE ALL ON TABLE portal.portal_content_versions FROM PUBLIC;

INSERT INTO portal.schema_migrations (version, name)
VALUES (8, 'portal_content_versions_and_trash')
ON CONFLICT (version) DO NOTHING;

COMMIT;
