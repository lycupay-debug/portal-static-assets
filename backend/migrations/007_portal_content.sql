BEGIN;

CREATE TABLE IF NOT EXISTS portal.portal_content (
    content_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id UUID NOT NULL REFERENCES portal.portal_users(user_id) ON DELETE RESTRICT,
    title TEXT NOT NULL,
    slug TEXT NOT NULL,
    content_type TEXT NOT NULL CHECK (content_type IN ('article', 'tool', 'resource', 'project')),
    summary TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL,
    visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('public', 'private')),
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'unpublished')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT portal_content_title_length CHECK (char_length(btrim(title)) BETWEEN 1 AND 200),
    CONSTRAINT portal_content_slug_format CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND char_length(slug) <= 100),
    CONSTRAINT portal_content_summary_length CHECK (char_length(summary) <= 500),
    CONSTRAINT portal_content_body_length CHECK (char_length(body) BETWEEN 1 AND 100000)
);

CREATE UNIQUE INDEX IF NOT EXISTS portal_content_slug_active_unique
    ON portal.portal_content (slug)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS portal_content_author_updated_idx
    ON portal.portal_content (author_id, updated_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS portal_content_public_published_idx
    ON portal.portal_content (published_at DESC)
    WHERE status = 'published' AND visibility = 'public' AND deleted_at IS NULL;

ALTER TABLE portal.portal_content OWNER TO personal_portal_app;
REVOKE ALL ON TABLE portal.portal_content FROM PUBLIC;

INSERT INTO portal.schema_migrations (version, name)
VALUES (7, 'portal_content')
ON CONFLICT (version) DO NOTHING;

COMMIT;
