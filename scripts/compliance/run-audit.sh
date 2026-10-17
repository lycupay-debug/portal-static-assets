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
chmod 0600 "$REPORT_DIR"/*
echo "Compliance inventory completed: $REPORT_DIR"
