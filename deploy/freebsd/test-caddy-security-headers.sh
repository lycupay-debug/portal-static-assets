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

# Old editor URLs must resolve to the integrated portal editor; the retired SPA must not be served.
for directive in \
  'handle /editor {' \
  'redir /content-management 308' \
  'handle /editor/assets/* {' \
  'respond 410' \
  'handle_path /editor/* {' \
  'redir /content-management{path} 308'; do
  if ! grep -Fq "$directive" "$config"; then
    echo "FAIL: missing legacy editor compatibility directive: $directive" >&2
    exit 1
  fi
done

# Replacing the production Caddyfile must preserve the existing GSM route.
if ! grep -Fq 'go.lycupaynotion.top {' "$config" || ! grep -Fq 'reverse_proxy 127.0.0.1:8766' "$config"; then
  echo "FAIL: the deployment Caddyfile must preserve the existing GSM reverse proxy." >&2
  exit 1
fi

echo "PASS: public security headers, Cloudflare client-IP trust, and GSM route preservation are configured."
