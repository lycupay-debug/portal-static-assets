# Go SSH MCP operation log integration

The account page `/account/system-ops/logs` reads its SSH/GPT feed from `/var/log/host-mcp-admin-audit.log` through the existing `/api/system-ops/logs?source=ssh` and `/api/system-ops/stream` endpoints.

The deployed `go_ssh_mcp_log_sync` service reads the root-owned `/var/log/kerno-audit.log` written by GSM/KERNO and continuously projects only an allowlisted set of operation events into the portal feed. It does not read a PostgreSQL event table: this deployment's source of truth is the KERNO JSON-lines audit log. The cursor stores source inode and byte offset in `/var/db/go-ssh-mcp-service-log.cursor`; a first run safely backfills existing audit entries, and later runs process only appended records or a newly rotated file.

The projection contains only the timestamp, operation type, outcome, duration, and command exit status where available. It deliberately excludes shell command text, raw command output, database URLs, access tokens, passwords, secret values, and arbitrary file paths. The raw audit log remains root-owned; the projected feed is `root:personalportal` mode `0640`, so the portal API can read it without exposing the source log.

FreeBSD service: `go_ssh_mcp_log_sync` (starts after `kerno`); poller: `/usr/local/sbin/go-ssh-mcp-log-sync`; projector: `/usr/local/sbin/go-ssh-mcp-service-log-sync.py`; service definition: `/usr/local/etc/rc.d/go_ssh_mcp_log_sync`. The poller checks for new entries once per second. The portal's existing file-tail/SSE code provides live UI updates.
