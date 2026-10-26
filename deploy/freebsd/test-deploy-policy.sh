#!/bin/sh
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
. "$repo/deploy/freebsd/deploy-policy.sh"

for release_mode in development stable production; do
  for auto_rollback in 0 1 true yes; do
    PERSONAL_PORTAL_RELEASE_MODE="$release_mode"
    PERSONAL_PORTAL_AUTO_ROLLBACK="$auto_rollback"
    export PERSONAL_PORTAL_RELEASE_MODE PERSONAL_PORTAL_AUTO_ROLLBACK
    if personal_portal_auto_rollback_enabled; then
      echo "FAIL: automatic rollback must remain disabled (mode=$release_mode opt-in=$auto_rollback)" >&2
      exit 1
    fi
  done
done

unset PERSONAL_PORTAL_RELEASE_MODE PERSONAL_PORTAL_AUTO_ROLLBACK
if personal_portal_auto_rollback_enabled; then
  echo 'FAIL: automatic rollback must be disabled when variables are unset' >&2
  exit 1
fi

echo 'PASS: automatic rollback is disabled for all environment configurations.'
