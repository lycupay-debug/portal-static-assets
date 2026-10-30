#!/bin/sh
set -eu

api="http://127.0.0.1:8081"
tmp="/tmp/personal-portal-health-$$"
trap 'rm -f "$tmp"' EXIT HUP INT TERM

expect_http() {
  url="$1"
  expected="$2"
  attempt=0
  while [ "$attempt" -lt 10 ]; do
    actual=$(curl --resolve www.lycupaynotion.top:443:127.0.0.1 --max-time 5 -sS -o "$tmp" -w '%{http_code}' "$url" 2>/dev/null || true)
    if [ "$actual" = "$expected" ]; then return 0; fi
    attempt=$((attempt + 1))
    sleep 1
  done
  echo "FAIL: $url returned HTTP $actual; expected $expected" >&2
  cat "$tmp" >&2
  exit 1
}

curl -fsS "$api/healthz" | grep -q '"status":"ok'
curl -fsS "$api/dbz" | grep -q '"status":"ok'
curl -fsS "$api/readyz" | grep -q '"status":"ready'
curl -fsS "$api/metrics" | grep -q 'personal_portal_events'
expect_http "https://www.lycupaynotion.top/" 200
expect_http "https://www.lycupaynotion.top/account" 200
expect_http "https://www.lycupaynotion.top/api/public/content" 200
expect_http "https://www.lycupaynotion.top/api/auth/me" 401
grep -q '"error"' "$tmp"
expect_http "https://www.lycupaynotion.top/api/resources" 401
grep -q '"error"' "$tmp"
expect_http "https://www.lycupaynotion.top/uploads/00000000-0000-4000-8000-000000000000.png" 308
expect_http "https://www.lycupaynotion.top/api/uploads/00000000-0000-4000-8000-000000000000.png" 404
expect_http "https://www.lycupaynotion.top/api/events" 404
expect_http "https://www.lycupaynotion.top/api/dbz" 404
expect_http "https://www.lycupaynotion.top/api/readyz" 404
expect_http "https://www.lycupaynotion.top/api/metrics" 404

# Validate the public static-site security headers against the local Caddy origin.
header_code=$(curl --resolve www.lycupaynotion.top:443:127.0.0.1 --max-time 5 -sS -I -o "$tmp" -w '%{http_code}' https://www.lycupaynotion.top/)
if [ "$header_code" != "200" ]; then
  echo "FAIL: security-header probe returned HTTP $header_code" >&2
  exit 1
fi
for header in \
  'X-Content-Type-Options: nosniff' \
  'X-Frame-Options: DENY' \
  'Referrer-Policy: strict-origin-when-cross-origin' \
  'Permissions-Policy: camera=(), microphone=(), geolocation=()'; do
  if ! grep -iqF "$header" "$tmp"; then
    echo "FAIL: missing public security header: $header" >&2
    cat "$tmp" >&2
    exit 1
  fi
done

node_enabled=$(sysrc -n personal_portal_auth_api_enable 2>/dev/null || echo NO)
if [ "$node_enabled" != "NO" ]; then
  echo "FAIL: retired Node.js API must be disabled in rc.conf" >&2
  exit 1
fi
if ps axww -o command | grep -E '[n]ode .*personal-portal(-legacy-node)?/backend/auth-api/server\.mjs' >/dev/null; then
  echo "FAIL: retired Node.js API process is still running" >&2
  exit 1
fi
echo "PASS: Go API, PostgreSQL, C event core, metrics, public portal, auth boundary, and internal-route isolation"
