#!/bin/sh

# Automatic rollback is intentionally disabled throughout development.
# Recovery must be an explicit operator decision; environment variables cannot
# re-enable automatic rollback. Keep the manual rollback script available.
personal_portal_auto_rollback_enabled() {
  return 1
}

# Production deployment/rollback stops and starts services. Require a separate,
# explicit operator acknowledgement rather than allowing an ordinary script run
# to restart the production stack.
personal_portal_require_service_restart_authorization() {
  if [ "${PERSONAL_PORTAL_ALLOW_SERVICE_RESTART:-0}" = "1" ]; then
    return 0
  fi
  echo "Service restart authorization is required. Set PERSONAL_PORTAL_ALLOW_SERVICE_RESTART=1 only after explicit operator approval." >&2
  return 1
}
