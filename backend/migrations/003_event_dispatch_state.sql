-- Durable dispatch lease and retry state for event inbox notifications.
ALTER TABLE portal.event_inbox
    ADD COLUMN IF NOT EXISTS dispatch_state text NOT NULL DEFAULT 'pending'
        CHECK (dispatch_state IN ('pending', 'leased', 'notified')),
    ADD COLUMN IF NOT EXISTS dispatch_attempts integer NOT NULL DEFAULT 0
        CHECK (dispatch_attempts >= 0),
    ADD COLUMN IF NOT EXISTS dispatch_lease_until timestamptz,
    ADD COLUMN IF NOT EXISTS next_dispatch_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS dispatch_notified_at timestamptz,
    ADD COLUMN IF NOT EXISTS dispatch_error text;

CREATE INDEX IF NOT EXISTS event_inbox_dispatch_due_idx
    ON portal.event_inbox (next_dispatch_at, received_at, event_id)
    WHERE dispatch_state = 'pending';

CREATE INDEX IF NOT EXISTS event_inbox_dispatch_lease_idx
    ON portal.event_inbox (dispatch_lease_until)
    WHERE dispatch_state = 'leased';

INSERT INTO portal.schema_migrations (version, name)
VALUES (3, 'event_dispatch_state')
ON CONFLICT (version) DO NOTHING;
