-- Processing lease and retry schedule are independent from dispatch notification state.
ALTER TABLE portal.event_inbox
    ADD COLUMN IF NOT EXISTS processing_lease_until timestamptz,
    ADD COLUMN IF NOT EXISTS processing_worker_id text,
    ADD COLUMN IF NOT EXISTS next_processing_at timestamptz NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS event_inbox_processing_due_idx
    ON portal.event_inbox (next_processing_at, received_at, event_id)
    WHERE status = 'received';

CREATE INDEX IF NOT EXISTS event_inbox_processing_lease_idx
    ON portal.event_inbox (processing_lease_until)
    WHERE status = 'processing';

INSERT INTO portal.schema_migrations (version, name)
VALUES (4, 'event_processing_leases')
ON CONFLICT (version) DO NOTHING;
