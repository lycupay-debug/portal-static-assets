#!/bin/sh
set -eu
if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root so the scan can read production releases and private resource storage." >&2
  exit 1
fi
ROOT=/opt/personal-portal
DB_URL_FILE=/usr/local/etc/personal-portal/database-url
WEBROOT=/usr/local/www/personal-portal
RESOURCE_DIR=/var/db/personal-portal/resources
REPORT_ROOT=/var/db/personal-portal/compliance-reports
if [ ! -r "$DB_URL_FILE" ]; then echo "Missing database URL file." >&2; exit 1; fi
DB_URL=$(cat "$DB_URL_FILE")
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
mkdir -p "$REPORT_ROOT"
chmod 0700 "$REPORT_ROOT"
REPORT_DIR="$REPORT_ROOT/$STAMP"
mkdir -p "$REPORT_DIR"
chmod 0700 "$REPORT_DIR"
WORK=$(mktemp -d /var/db/personal-portal/compliance-run.XXXXXX)
chmod 0700 "$WORK"
trap 'rm -rf "$WORK"' EXIT INT TERM
DB_JSON="$WORK/database-snapshot.json"
cat > "$WORK/inventory.sql" <<'SQL'
SELECT jsonb_build_object(
  'content', COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'id',content_id::text,'title',title,'slug',slug,'type',content_type,
      'status',status,'visibility',visibility,'body',body,'summary',summary
    ) ORDER BY created_at)
    FROM portal.portal_content WHERE deleted_at IS NULL
  ), '[]'::jsonb),
  'resources', COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'id',resource_id::text,'title',title,'description',description,
      'original_filename',original_filename,'stored_filename',stored_filename,
      'mime_type',mime_type,'file_size',file_size,'resource_type',resource_type,
      'resource_url',resource_url,'is_public',is_public
    ) ORDER BY created_at)
    FROM portal.teaching_resources WHERE deleted_at IS NULL
  ), '[]'::jsonb)
);
SQL
psql "$DB_URL" -X -A -t -v ON_ERROR_STOP=1 -f "$WORK/inventory.sql" > "$DB_JSON"
chmod 0600 "$DB_JSON"
python3.14 "$ROOT/scripts/compliance/scan_assets.py" \
  --repo "$ROOT" --db-json "$DB_JSON" \
  --release "$WEBROOT/current" --uploads "$WEBROOT/uploads" \
  --resources "$RESOURCE_DIR" --out "$REPORT_DIR"
psql "$DB_URL" -X -q -v ON_ERROR_STOP=1 -f "$REPORT_DIR/seed-compliance-inventory.sql"
# Retention review is intentionally report-only. It never deletes data; complaint
# retention and legal-hold rules still require an approved policy.
psql "$DB_URL" -X -A -t -v ON_ERROR_STOP=1 -f "$ROOT/scripts/compliance/retention-dry-run.sql" > "$REPORT_DIR/retention-dry-run.json"
UPLOAD_FILES=$(find "$WEBROOT/uploads" -maxdepth 1 -type f 2>/dev/null | wc -l | tr -d ' ')
REGISTERED_UPLOADS=$(psql "$DB_URL" -X -A -t -v ON_ERROR_STOP=1 -c "SELECT count(*) FROM portal.uploaded_image_assets")
{
  echo "mode=report_only"
  echo "files_on_disk=$UPLOAD_FILES"
  echo "registered_image_asset_rows=$REGISTERED_UPLOADS"
  echo "delete_actions=none"
} > "$REPORT_DIR/upload-filesystem-inventory.txt"
BACKUP_DIR=/var/backups/personal-portal
BACKUP_FILES=$(find "$BACKUP_DIR" -maxdepth 1 -type f 2>/dev/null | wc -l | tr -d ' ')
OLDEST_BACKUP_EPOCH=$(find "$BACKUP_DIR" -maxdepth 1 -type f -exec stat -f '%m' {} \; 2>/dev/null | sort -n | head -1)
{
  echo "mode=report_only"
  echo "backup_files=$BACKUP_FILES"
  if [ -n "$OLDEST_BACKUP_EPOCH" ]; then
    echo "oldest_backup_mtime=$(date -u -r "$OLDEST_BACKUP_EPOCH" '+%Y-%m-%dT%H:%M:%SZ')"
  else
    echo "oldest_backup_mtime=none"
  fi
  echo "automatic_backup_deletion=not_configured_by_this_job"
} > "$REPORT_DIR/backup-retention-inventory.txt"
chmod 0600 "$REPORT_DIR"/*
echo "Compliance inventory and retention dry-run completed: $REPORT_DIR"
