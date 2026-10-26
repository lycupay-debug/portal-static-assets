#!/bin/sh

# Automatic rollback is intentionally disabled throughout development.
# Recovery must be an explicit operator decision; environment variables cannot
# re-enable automatic rollback. Keep the manual rollback script available.
personal_portal_auto_rollback_enabled() {
  return 1
}
