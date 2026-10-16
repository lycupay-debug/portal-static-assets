#!/bin/sh
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
config="$repo/deploy/freebsd/Caddyfile"

for directive in \
  'X-Content-Type-Options "nosniff"' \
  'X-Frame-Options "DENY"' \
  'Referrer-Policy "strict-origin-when-cross-origin"' \
  'Permissions-Policy "camera=(), microphone=(), geolocation=()"'; do
  if ! grep -Fq "$directive" "$config"; then
    echo "FAIL: missing public static-site security header: $directive" >&2
    exit 1
  fi
done

echo "PASS: public static-site security headers are configured."
