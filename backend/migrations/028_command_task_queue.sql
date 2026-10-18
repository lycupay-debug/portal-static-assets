BEGIN;
CREATE TABLE IF NOT EXISTS portal.command_tasks (
    task_id UUID PRIMARY KEY,
    idempotency_key TEXT NOT NULL UNIQUE CHECK (length(idempotency_key) BETWEEN 8 AND 160),
    title TEXT NOT NULL CHECK (length(title) BETWEEN 3 AND 160),
    task_class TEXT NOT NULL CHECK (task_class IN ('read_only','file_inspection','test','build','code_change','system_admin','deploy','restart','database_change','destructive','ssh_mcp')),
    executable TEXT NOT NULL CHECK (length(executable) BETWEEN 1 AND 512 AND executable LIKE '/%'),
    arguments JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(arguments) = 'array'),
    working_directory TEXT NOT NULL CHECK (working_directory LIKE '/%'),
    shell_mode BOOLEAN NOT NULL DEFAULT FALSE,
    timeout_seconds INTEGER NOT NULL DEFAULT 120 CHECK (timeout_seconds BETWEEN 1 AND 1800),
    requested_by TEXT NOT NULL,
    mode_at_submission TEXT NOT NULL CHECK (mode_at_submission IN ('developer','stable')),
    status TEXT NOT NULL DEFAULT 'pending_approval' CHECK (status IN ('pending_approval','queued','running','completed','failed','cancelled','manual_only','denied')),
    approval_required BOOLEAN NOT NULL DEFAULT TRUE,
    approved_by TEXT,
    approved_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    exit_code INTEGER,
    stdout TEXT NOT NULL DEFAULT '',
    stderr TEXT NOT NULL DEFAULT '',
    worker_id TEXT,
    lease_until TIMESTAMPTZ,
    process_id INTEGER,
    attempt INTEGER NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    cancel_requested BOOLEAN NOT NULL DEFAULT FALSE,
    failure_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((approved_at IS NULL) = (approved_by IS NULL)),
    CHECK (status <> 'running' OR (worker_id IS NOT NULL AND lease_until IS NOT NULL AND started_at IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS command_tasks_queue_idx ON portal.command_tasks(created_at, task_id) WHERE status = 'queued';
CREATE INDEX IF NOT EXISTS command_tasks_lease_idx ON portal.command_tasks(lease_until) WHERE status = 'running';
CREATE INDEX IF NOT EXISTS command_tasks_recent_idx ON portal.command_tasks(created_at DESC, task_id DESC);
CREATE TABLE IF NOT EXISTS portal.command_executor_heartbeat (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton = TRUE),
    worker_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'offline' CHECK (status IN ('ready','stopping','offline','error')),
    started_at TIMESTAMPTZ,
    last_seen_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT ''
);
INSERT INTO portal.command_executor_heartbeat(singleton,worker_id,status) VALUES(TRUE,'','offline') ON CONFLICT (singleton) DO NOTHING;
ALTER TABLE portal.command_executor_heartbeat OWNER TO personal_portal_app;
ALTER TABLE portal.command_tasks OWNER TO personal_portal_app;
REVOKE ALL ON TABLE portal.command_tasks FROM PUBLIC;
INSERT INTO portal.schema_migrations(version, name)
VALUES (28, 'command_task_queue')
ON CONFLICT (version) DO NOTHING;
COMMIT;
