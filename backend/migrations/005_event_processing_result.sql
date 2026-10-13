-- Store handler output atomically with the event's completed state.
ALTER TABLE portal.event_inbox
    ADD COLUMN IF NOT EXISTS processing_result jsonb;

INSERT INTO portal.schema_migrations (version, name)
VALUES (5, 'event_processing_result')
ON CONFLICT (version) DO NOTHING;
