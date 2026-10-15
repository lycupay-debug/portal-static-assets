BEGIN;
CREATE TABLE IF NOT EXISTS portal.portal_content_interactions (
    interaction_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id UUID NOT NULL REFERENCES portal.portal_content(content_id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES portal.portal_users(user_id) ON DELETE CASCADE,
    interaction_type TEXT NOT NULL CHECK (interaction_type IN ('like', 'favorite')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT portal_content_interactions_unique UNIQUE (content_id, user_id, interaction_type)
);
CREATE INDEX IF NOT EXISTS portal_content_interactions_content_type_idx
    ON portal.portal_content_interactions (content_id, interaction_type);
ALTER TABLE portal.portal_content_interactions OWNER TO personal_portal_app;
REVOKE ALL ON TABLE portal.portal_content_interactions FROM PUBLIC;
INSERT INTO portal.schema_migrations (version, name)
VALUES (10, 'portal_content_interactions')
ON CONFLICT (version) DO NOTHING;
COMMIT;
