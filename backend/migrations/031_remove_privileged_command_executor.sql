-- Retire the privileged command-task executor completely. Historical migrations 027-030 remain immutable.
-- The old root worker must be explicitly stopped by the deployment retirement gate before this migration runs.
BEGIN;

-- Do not dispatch stale wake notifications after the worker has been removed.
DELETE FROM portal.event_inbox WHERE event_type = 'portal.command.execute';

-- These tables contain task payloads/output and the persisted execution-mode switch.
DROP TABLE IF EXISTS portal.command_tasks;
DROP TABLE IF EXISTS portal.command_executor_heartbeat;
DROP TABLE IF EXISTS portal.system_execution_policy;

-- System operations now expose read-only logs/warnings; RBAC management remains separate.
DELETE FROM portal.role_permissions WHERE permission_key = 'portal.system_ops.manage';
DELETE FROM portal.permissions WHERE permission_key = 'portal.system_ops.manage';

INSERT INTO portal.schema_migrations(version, name)
VALUES (31, 'remove_privileged_command_executor')
ON CONFLICT (version) DO NOTHING;

COMMIT;
