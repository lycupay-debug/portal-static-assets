#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root." >&2
  exit 1
fi
if [ "${PERSONAL_PORTAL_RETIRE_COMMAND_EXECUTOR:-0}" != "1" ]; then
  echo "Set PERSONAL_PORTAL_RETIRE_COMMAND_EXECUTOR=1 to authorize stopping and removing the legacy root executor." >&2
  exit 2
fi

service_file="/usr/local/etc/rc.d/personal_portal_task_executor"
if [ -x "$service_file" ] && service personal_portal_task_executor status >/dev/null 2>&1; then
  service personal_portal_task_executor stop
fi
sysrc personal_portal_task_executor_enable=NO >/dev/null 2>&1 || true

# Never continue to the destructive database migration while an orphaned root worker remains.
if pgrep -f '[/]usr/local/libexec/personal-portal/api --task-executor' >/dev/null 2>&1; then
  echo "A legacy task-executor process is still running; inspect and stop it manually before continuing." >&2
  exit 1
fi

rm -f "$service_file"
rm -rf /var/run/personal-portal-task-executor /var/db/personal-portal/task-home
# Keep /var/log/personal-portal/task-executor.log and existing backups as historical audit evidence.
echo "Legacy command executor stopped, disabled, and decommissioned."
