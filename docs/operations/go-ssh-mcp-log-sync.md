# GO_SSH_MCP operation log integration

The account page `/account/system-ops/logs` reads its SSH/GPT feed from `/var/log/host-mcp-admin-audit.log` through the existing `/api/system-ops/logs?source=ssh` and `/api/system-ops/stream` endpoints.

`deploy/freebsd/go-ssh-mcp-log-sync` polls the GO_SSH_MCP PostgreSQL `go_ssh_mcp.events` table and writes a sanitized JSON-lines projection to that existing feed. It starts with the current maximum event ID and only projects subsequent events, so old operation history is not replayed after the log reset. Cursor state is stored at `/var/db/go-ssh-mcp-log-sync.cursor`.

The projection includes lifecycle event type, outcome, timestamps, task/batch identifiers, operation name and duration. It deliberately excludes shell command text, raw command output, result payloads, access tokens and secrets. The service runs as root to read the GO_SSH_MCP connection configuration and writes the projection as `root:personalportal` mode `0640`, allowing the portal API to read it without exposing the raw root-only audit log.

FreeBSD service: `go_ssh_mcp_log_sync`; implementation: `/usr/local/sbin/go-ssh-mcp-log-sync`; service definition: `/usr/local/etc/rc.d/go_ssh_mcp_log_sync`. The loop polls once per second. The portal's existing file-tail/SSE code provides the live UI updates.
