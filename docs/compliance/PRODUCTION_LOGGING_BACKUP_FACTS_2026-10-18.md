# Production logging and backup fact review — 2026-10-18

Scope: read-only inspection of the FreeBSD host, the active Caddy configuration, PostgreSQL runtime settings, newsyslog configuration, root cron, and backup/report directory metadata. This records observed configuration only; it is not a legal conclusion or a complete Cloudflare/provider audit.

## Observed facts

| Area | Observed configuration | What this does and does not establish |
|---|---|---|
| PostgreSQL logging | `log_destination=syslog`; `logging_collector=off`; `log_connections=off`; `log_disconnections=off`; `log_statement=none`; `log_min_duration_statement=-1`; prefix `%m [%p] ` | PostgreSQL sends its configured logs to syslog. Statement logging and connection/disconnection logging are disabled in the inspected runtime settings. This does not prove that application, operating-system, proxy, or provider logs contain no personal data. |
| System log rotation | `/etc/newsyslog.conf` contains `/var/log/messages 644 5 1000 @0101T JC` | A rotation rule exists for `/var/log/messages`. The effective retention period and actual rotated-file lifecycle still need an operational test; do not translate this raw rule into a precise retention promise without validating it. |
| Caddy access logs | The active Caddyfile has no `log` directive in the inspected site blocks. | This configuration does not explicitly enable Caddy access logging for these sites. It does not rule out upstream Cloudflare logs or other host-level records. |
| Compliance audit log | `/var/log/personal-portal-compliance-audit.log` exists as `root:wheel`, mode `0600`; daily cron invokes `run-audit.sh` at 03:30. | The file is currently empty at inspection time. The cron command redirects standard output/error there. |
| Compliance reports | `/var/db/personal-portal/compliance-reports` is `root:wheel`, mode `0700`; generated report files are mode `0600`. | Reports are restricted to root at the filesystem permission layer. |
| Backups | `/var/backups/personal-portal` is `root:wheel`, mode `0700`; recent database dumps are mode `0600`. Multiple dated deployment dumps and frontend pointers exist. | No automated backup-retention/deletion job was found in the inspected root crontab or reviewed deployment scripts. Historical backups may retain deleted content and personal data. Do not delete them until a retention schedule and restore requirements are approved. |
| Cloudflare / upstream providers | Not fully inspected in this host-only review. | Data region, edge logging, provider retention, backups, and cross-border processing remain unverified and must not be stated as facts in a public privacy policy. |

## Follow-up controls

1. Validate log rotation by checking the actual rotated files and a safe test rotation; do not change production log policy based only on this inventory.
2. Decide a backup retention schedule that accounts for database recovery, legal holds, and data deletion requests. Implement reporting first, then an explicit reviewed cleanup procedure.
3. Inspect Cloudflare account settings and any connected provider/backup services before publishing factual claims about location, access, or retention.
4. Keep public legal documents in draft until the responsible operator confirms these facts and the legal review is complete.
