#!/bin/sh
# Validate only the current public development portal and its isolated API.
set -eu

api="http://127.0.0.1:8082"
tmp="/tmp/portal-static-assets-dev-health-$$"
trap 'rm -f "$tmp" "$tmp.headers"' EXIT HUP INT TERM

expect_http() {
  path="$1"
  expected="$2"
  actual=$(curl --resolve www.lycupaynotion.top:443:127.0.0.1 --max-time 8 -sS -o "$tmp" -w '%{http_code}' "https://www.lycupaynotion.top$path")
  if [ "$actual" != "$expected" ]; then
    echo "FAIL: $path returned HTTP $actual; expected $expected" >&2
    cat "$tmp" >&2
    exit 1
  fi
}

curl -fsS "$api/healthz" | grep -q '"status":"ok'
curl -fsS "$api/dbz" | grep -q '"status":"ok'
curl -fsS "$api/readyz" | grep -q '"status":"ready'
curl -fsS "$api/metrics" | grep -q 'personal_portal_events'
expect_http / 200
expect_http /account 200
expect_http /api/public/content 200
expect_http /api/auth/me 401
expect_http /api/resources 401
legacy_image_headers="$tmp.headers"
legacy_image_code=$(curl --resolve www.lycupaynotion.top:443:127.0.0.1 --max-time 8 -sS -D "$legacy_image_headers" -o "$tmp" -w '%{http_code}' https://www.lycupaynotion.top/uploads/00000000-0000-4000-8000-000000000000.png)
if [ "$legacy_image_code" != "308" ] || ! grep -iqF 'Location: /api/uploads/00000000-0000-4000-8000-000000000000.png' "$legacy_image_headers"; then
  echo "FAIL: legacy image URL must redirect to the cookie-covered API path (HTTP 308)." >&2
  cat "$legacy_image_headers" >&2
  exit 1
fi
expect_http /api/uploads/00000000-0000-4000-8000-000000000000.png 404
upload_unauth_code=$(curl --resolve www.lycupaynotion.top:443:127.0.0.1 --max-time 8 -sS -o "$tmp" -w '%{http_code}' \
  -X POST -H 'Content-Type: application/json' --data '{}' \
  https://www.lycupaynotion.top/api/uploads)
if [ "$upload_unauth_code" != "401" ]; then
  echo "FAIL: unauthenticated image upload returned HTTP $upload_unauth_code; expected 401." >&2
  cat "$tmp" >&2
  exit 1
fi
expect_http /api/events 404
expect_http /api/dbz 404
expect_http /api/readyz 404
expect_http /api/metrics 404

registration_code=$(curl --resolve www.lycupaynotion.top:443:127.0.0.1 --max-time 8 -sS -o "$tmp" -w '%{http_code}' \
  -X POST -H 'Content-Type: application/json' --data '{}' \
  https://www.lycupaynotion.top/api/auth/register)
if [ "$registration_code" != "403" ]; then
  echo "FAIL: public registration is not disabled (HTTP $registration_code)." >&2
  cat "$tmp" >&2
  exit 1
fi

api_pid=$(cat /var/run/portal-static-assets-dev/api.pid)
if ! procstat -e "$api_pid" | grep -q 'PERSONAL_PORTAL_REGISTRATION_ENABLED=0'; then
  echo "FAIL: development API was not started with registration disabled." >&2
  exit 1
fi
for directory in /var/run/portal-static-assets-dev /var/db/portal-static-assets-dev/uploads /var/db/portal-static-assets-dev/resources; do
  owner=$(stat -f '%Su:%Sg' "$directory")
  mode=$(stat -f '%Lp' "$directory")
  if [ "$owner" != "personalportal:personalportal" ] || [ "$mode" != "750" ]; then
    echo "FAIL: unsafe development storage permissions on $directory: $owner $mode" >&2
    exit 1
  fi
done

headers=$(curl --resolve www.lycupaynotion.top:443:127.0.0.1 --max-time 8 -sS -I https://www.lycupaynotion.top/)
printf '%s\n' "$headers" | grep -iqF 'X-Content-Type-Options: nosniff'
printf '%s\n' "$headers" | grep -iqF 'X-Frame-Options: DENY'
printf '%s\n' "$headers" | grep -iqF 'Referrer-Policy: strict-origin-when-cross-origin'
printf '%s\n' "$headers" | grep -iqF 'Permissions-Policy: camera=(), microphone=(), geolocation=()'

service caddy status >/dev/null
echo "PASS: development API, PostgreSQL, C event core, public routes, image ownership gateway, registration closure, storage permissions, and security headers"
