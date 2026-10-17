#!/bin/sh
set -eu
DB_URL_FILE=/usr/local/etc/personal-portal/database-url
if printenv PERSONAL_PORTAL_DATABASE_URL_FILE >/dev/null 2>&1; then DB_URL_FILE=$(printenv PERSONAL_PORTAL_DATABASE_URL_FILE); fi
if [ ! -r "$DB_URL_FILE" ]; then
  echo "Database URL file is not readable: $DB_URL_FILE" >&2
  exit 1
fi
DB_URL=$(cat "$DB_URL_FILE")
ACTION=list
if [ "$#" -ge 1 ]; then ACTION=$1; fi
case "$ACTION" in
  list)
    psql "$DB_URL" -X -P pager=off -c "SELECT user_id::text, display_name, left(split_part(email,'@',1),2)||'***@'||split_part(email,'@',2) AS masked_email, is_admin, created_at FROM portal.portal_users ORDER BY created_at;"
    ;;
  grant)
    USER_ID=""
    CONFIRM=""
    if [ "$#" -ge 2 ]; then USER_ID=$2; fi
    if [ "$#" -ge 3 ]; then CONFIRM=$3; fi
    if [ -z "$USER_ID" ] || [ "$CONFIRM" != "GRANT" ]; then
      echo "Usage: $0 grant <exact-user-uuid> GRANT" >&2
      echo "First run '$0 list', verify the account, then type GRANT explicitly." >&2
      exit 2
    fi
    psql "$DB_URL" -X -v ON_ERROR_STOP=1 -v user_id="$USER_ID" -c "UPDATE portal.portal_users SET is_admin=TRUE WHERE user_id=:'user_id'::uuid RETURNING user_id::text,display_name,left(split_part(email,'@',1),2)||'***@'||split_part(email,'@',2) AS masked_email,is_admin;"
    ;;
  revoke)
    USER_ID=""
    CONFIRM=""
    if [ "$#" -ge 2 ]; then USER_ID=$2; fi
    if [ "$#" -ge 3 ]; then CONFIRM=$3; fi
    if [ -z "$USER_ID" ] || [ "$CONFIRM" != "REVOKE" ]; then
      echo "Usage: $0 revoke <exact-user-uuid> REVOKE" >&2
      exit 2
    fi
    psql "$DB_URL" -X -v ON_ERROR_STOP=1 -v user_id="$USER_ID" -c "UPDATE portal.portal_users SET is_admin=FALSE WHERE user_id=:'user_id'::uuid RETURNING user_id::text,display_name,left(split_part(email,'@',1),2)||'***@'||split_part(email,'@',2) AS masked_email,is_admin;"
    ;;
  *)
    echo "Actions: list | grant <uuid> GRANT | revoke <uuid> REVOKE" >&2
    exit 2
    ;;
esac
