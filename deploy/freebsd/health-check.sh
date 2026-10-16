#!/bin/sh
set -eu

api="http://127.0.0.1:8081"
tmp="/tmp/personal-portal-health-$$"
trap 'rm -f "$tmp"' EXIT HUP INT TERM

expect_http() {
  url="$1"
  expected="$2"
  actual=$(curl --resolve www.lycupaynotion.top:443:127.0.0.1 -sS -o "$tmp" -w '%{http_code}' "$url")
  if [ "$actual" != "$expected" ]; then
    echo "FAIL: $url returned HTTP $actual; expected $expected" >&2
    cat "$tmp" >&2
    exit 1
  fi
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
expect_http "https://www.lycupaynotion.top/api/events" 404
expect_http "https://www.lycupaynotion.top/api/dbz" 404
expect_http "https://www.lycupaynotion.top/api/readyz" 404
expect_http "https://www.lycupaynotion.top/api/metrics" 404

node_enabled=$(sysrc -n personal_portal_auth_api_enable 2>/dev/null || echo NO)
if [ "$node_enabled" != "NO" ]; then
  echo "FAIL: retired Node.js API must be disabled in rc.conf" >&2
  exit 1
fi
echo "PASS: Go-only API, PostgreSQL, C event core, metrics, public portal, auth boundary, and internal-route isolation"
