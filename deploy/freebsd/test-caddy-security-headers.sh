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

for directive in \
  'trusted_proxies static' \
  'trusted_proxies_strict' \
  'client_ip_headers CF-Connecting-IP' \
  'header_up X-Portal-Client-IP {client_ip}'; do
  if ! grep -Fq "$directive" "$config"; then
    echo "FAIL: missing trusted Cloudflare client-IP configuration: $directive" >&2
    exit 1
  fi
done

echo "PASS: public security headers and Cloudflare client-IP trust are configured."
