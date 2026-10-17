#!/bin/sh
set -eu
if [ "$(id -u)" -ne 0 ]; then echo "Run as root." >&2; exit 1; fi
ENTRY='30 3 * * * /opt/personal-portal/scripts/compliance/run-audit.sh >> /var/log/personal-portal-compliance-audit.log 2>&1'
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
BACKUP_DIR=/var/backups/personal-portal
mkdir -p "$BACKUP_DIR"
chmod 0700 "$BACKUP_DIR"
TMP=$(mktemp /var/tmp/personal-portal-root-crontab.XXXXXX)
trap 'rm -f "$TMP"' EXIT INT TERM
crontab -l > "$TMP" 2>/dev/null || true
cp "$TMP" "$BACKUP_DIR/root-crontab-before-compliance-$STAMP"
chmod 0600 "$BACKUP_DIR/root-crontab-before-compliance-$STAMP"
if grep -Fq '/opt/personal-portal/scripts/compliance/run-audit.sh' "$TMP"; then
  echo "A compliance audit cron entry already exists; leaving crontab unchanged."
  exit 0
fi
printf '%s\n' "$ENTRY" >> "$TMP"
crontab "$TMP"
touch /var/log/personal-portal-compliance-audit.log
chown root:wheel /var/log/personal-portal-compliance-audit.log
chmod 0600 /var/log/personal-portal-compliance-audit.log
echo "Installed daily 03:30 compliance inventory. Crontab backup: $BACKUP_DIR/root-crontab-before-compliance-$STAMP"
