BEGIN;

CREATE TABLE IF NOT EXISTS portal.portal_users (
    user_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL,
    display_name TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT portal_users_email_lowercase CHECK (email = lower(email)),
    CONSTRAINT portal_users_email_length CHECK (char_length(email) BETWEEN 3 AND 254),
    CONSTRAINT portal_users_display_name_length CHECK (char_length(display_name) BETWEEN 1 AND 60)
);

CREATE UNIQUE INDEX IF NOT EXISTS portal_users_email_unique ON portal.portal_users (email);

CREATE TABLE IF NOT EXISTS portal.portal_auth_sessions (
    session_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES portal.portal_users(user_id) ON DELETE CASCADE,
    token_hash CHAR(64) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT portal_auth_sessions_expiry CHECK (expires_at > created_at)
);

CREATE INDEX IF NOT EXISTS portal_auth_sessions_user_id_idx ON portal.portal_auth_sessions (user_id);
CREATE INDEX IF NOT EXISTS portal_auth_sessions_expires_at_idx ON portal.portal_auth_sessions (expires_at);

INSERT INTO portal.schema_migrations (version, name, applied_at)
VALUES (6, 'portal_auth', now())
ON CONFLICT (version) DO NOTHING;

COMMIT;
