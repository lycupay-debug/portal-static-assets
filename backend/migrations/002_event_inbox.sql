-- Durable event intake for the C event-driven portal architecture.
CREATE TABLE IF NOT EXISTS portal.event_inbox (
    event_id uuid PRIMARY KEY,
    event_type text NOT NULL CHECK (event_type ~ '^[a-z][a-z0-9_.-]{0,99}$'),
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    received_at timestamptz NOT NULL DEFAULT now(),
    status text NOT NULL DEFAULT 'received'
        CHECK (status IN ('received', 'processing', 'completed', 'failed')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error text,
    processed_at timestamptz
);

CREATE INDEX IF NOT EXISTS event_inbox_received_idx
    ON portal.event_inbox (received_at, event_id)
    WHERE status = 'received';

ALTER TABLE portal.event_inbox OWNER TO personal_portal_app;
REVOKE ALL ON TABLE portal.event_inbox FROM PUBLIC;

INSERT INTO portal.schema_migrations (version, name)
VALUES (2, 'event_inbox')
ON CONFLICT (version) DO NOTHING;
