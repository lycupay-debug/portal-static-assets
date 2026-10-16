#!/bin/sh

# Development is the safe default. Automatic rollback requires both an
# explicit stable-phase declaration and an explicit opt-in.
personal_portal_auto_rollback_enabled() {
  [ "${PERSONAL_PORTAL_RELEASE_MODE:-development}" = "stable" ] &&
    [ "${PERSONAL_PORTAL_AUTO_ROLLBACK:-0}" = "1" ]
}
