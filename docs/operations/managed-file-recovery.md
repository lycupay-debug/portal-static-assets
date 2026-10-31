# Managed File Integrity and Recovery Runbook

## Scope

This runbook covers files stored by the portal API in the managed image and teaching-resource directories. PostgreSQL rows and filesystem objects cannot participate in one atomic transaction. A client disconnect or ambiguous PostgreSQL COMMIT result can therefore leave a file on disk without a committed row. The API deliberately retains the file when commit outcome is uncertain; it must not guess that the file is safe to delete.

This procedure is for the development environment by default. It does not authorize changes to production data.

## 1. Read-only integrity audit

Run the audit from the repository root:

```sh
sh scripts/audit-managed-files.sh
```

Override the database and managed directories only when the target environment is known:

```sh
PORTAL_DB_NAME=personal_portal_dev \
PORTAL_UPLOAD_DIR=/var/db/portal-static-assets-dev/uploads \
PORTAL_RESOURCE_DIR=/var/db/portal-static-assets-dev/resources \
sh scripts/audit-managed-files.sh
```

Exit status:

- `0`: database registrations and on-disk files match; no unexpected links or nested directories were found.
- `2`: one or more integrity anomalies were found. The report distinguishes missing registered files, unregistered disk files, symlinks, and nested directories.
- Other non-zero status: the audit could not complete (for example, PostgreSQL query failure or invalid managed directory).

The audit is read-only. It does not delete, move, rewrite, chmod, or otherwise modify managed files. An unregistered file is a finding, not proof that it is disposable.

## 2. Preserve evidence before investigating

For any non-zero audit:

1. Record the timestamp, command, exit status, full output, repository commit, and target database.
2. Create a fresh database backup using the approved development backup procedure; verify the dump with `pg_restore -l` and SHA-256.
3. Create a separate archive of both managed directories and verify its SHA-256. Do not place the archive inside either managed directory.
4. Do not stop services solely to run the read-only audit. If concurrent uploads are suspected, repeat the audit after the in-flight request window and compare both reports.
5. Do not run a cleanup command or delete an unregistered file.

## 3. Classify each finding

### Unregistered disk file

For each filename reported under `UNREGISTERED_DISK_FILES`:

1. Confirm the file is directly inside the expected managed directory and is not a symlink.
2. Search the relevant database table by the exact filename. Also inspect soft-deleted rows, active event-inbox cleanup events, and any application records that reference the stored filename.
3. Check recent API logs and the audit timestamp for an in-flight upload or ambiguous commit.
4. Compare the file with the verified backup and record its SHA-256, byte size, ownership, and modification time.
5. If any reference or transaction state remains uncertain, preserve the file and escalate; do not quarantine or delete it.

If the evidence proves that no live or historical business record needs the file, preserve it in a separately named quarantine archive only after review. Copy first, verify the copied SHA-256, and retain the original until a later review explicitly approves its removal. Quarantine is not an automatic part of the audit.

### Missing registered file

For each filename reported under `MISSING_REGISTERED_FILES`:

1. Identify the owning image/resource row and its visibility, deletion, and retention state.
2. Search verified backups for the exact original file.
3. Restore only a matching original file; verify its SHA-256 against the backup copy and verify ownership/mode against the managed-directory policy.
4. Re-run the read-only audit and exercise the relevant authorized read/download path.
5. If no verified original exists, record the affected row and escalate. Do not create a placeholder file or silently delete the database row.

### Symlink or nested directory

Treat these as unexpected filesystem structure. Record the exact path, target (for symlinks), ownership, and metadata. Do not follow the link or recursively modify the directory during triage. Verify the canonical managed-directory path and compare against backups before any reviewed remediation.

## 4. Completion evidence

A repair is complete only when all of the following are recorded:

- Before/after audit output and exit status.
- Database backup and managed-directory archive identifiers plus SHA-256 values.
- Exact filenames and database rows affected.
- Any restored or quarantined file SHA-256 and metadata.
- Successful post-repair audit.
- Relevant authenticated read/download test result.
- Reviewer, timestamp, and a risk-register update.

## Safety boundary

There is intentionally no automatic orphan-file deletion or bulk cleanup mode. Never infer that a file is disposable solely because its filename is absent from one query result. Keep the audit read-only and preserve ambiguous files until the database, event state, request history, and verified backups have been reconciled.
