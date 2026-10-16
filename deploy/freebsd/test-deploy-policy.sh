#!/bin/sh
set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
. "$repo/deploy/freebsd/deploy-policy.sh"

unset PERSONAL_PORTAL_RELEASE_MODE PERSONAL_PORTAL_AUTO_ROLLBACK
if personal_portal_auto_rollback_enabled; then
  echo 'FAIL: automatic rollback must be disabled by default' >&2
  exit 1
fi

PERSONAL_PORTAL_RELEASE_MODE=development
PERSONAL_PORTAL_AUTO_ROLLBACK=1
export PERSONAL_PORTAL_RELEASE_MODE PERSONAL_PORTAL_AUTO_ROLLBACK
if personal_portal_auto_rollback_enabled; then
  echo 'FAIL: auto rollback must remain disabled during development' >&2
  exit 1
fi

PERSONAL_PORTAL_RELEASE_MODE=stable
PERSONAL_PORTAL_AUTO_ROLLBACK=0
export PERSONAL_PORTAL_RELEASE_MODE PERSONAL_PORTAL_AUTO_ROLLBACK
if personal_portal_auto_rollback_enabled; then
  echo 'FAIL: stable mode alone must not enable auto rollback' >&2
  exit 1
fi

PERSONAL_PORTAL_RELEASE_MODE=stable
PERSONAL_PORTAL_AUTO_ROLLBACK=1
export PERSONAL_PORTAL_RELEASE_MODE PERSONAL_PORTAL_AUTO_ROLLBACK
if ! personal_portal_auto_rollback_enabled; then
  echo 'FAIL: explicit stable-phase opt-in should enable auto rollback' >&2
  exit 1
fi

echo 'PASS: automatic rollback requires both explicit stable mode and explicit opt-in.'
