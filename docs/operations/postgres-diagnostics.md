# PostgreSQL diagnostics

Run `scripts/ops/postgres-diagnostics.sh` on the FreeBSD portal host for read-only
schema, index-health, and index-usage checks. It reads the protected connection
URL from `/usr/local/etc/personal-portal/database-url` (or
`PERSONAL_PORTAL_DATABASE_URL_FILE`) so it does not accidentally connect using
`root` or to an inferred database such as `app_core`.

Important PostgreSQL catalog distinctions:

- `pg_stat_user_tables` reports table statistics and does **not** expose
  `idx_tup_read`; use `pg_stat_user_indexes` for `idx_scan`, `idx_tup_read`, and
  `idx_tup_fetch`.
- `pg_stat_user_indexes` uses `relname` and `indexrelname`; `pg_indexes` uses
  `tablename`, `indexname`, and `indexdef`.
- `grade` is a column of `portal.teaching_resources`, not `portal.portal_content`.
- When concatenating PostgreSQL's internal `"char"` catalog columns with text,
  cast them explicitly to `text`.
- Do not create speculative expression indexes from an ad-hoc shell query.
  PostgreSQL requires every function in an index expression to be immutable;
  use the versioned migrations and validate them against the isolated test DB
  before deploying.

The historical system log is retained for auditability. Correcting a diagnostic
query prevents recurrence; it does not rewrite the record of the earlier failed
query.
