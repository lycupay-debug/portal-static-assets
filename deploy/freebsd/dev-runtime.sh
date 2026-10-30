#!/bin/sh
# Manage only the isolated portal development API and C event-core processes.
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root." >&2
  exit 1
fi

command_name="${1:-status}"
run_dir="/var/run/portal-static-assets-dev"
bin_dir="/usr/local/libexec/portal-static-assets-dev"
log_dir="/var/log/portal-static-assets-dev"
storage_dir="/var/db/portal-static-assets-dev"
upload_dir="$storage_dir/uploads"
resource_dir="$storage_dir/resources"
database_url_file="/usr/local/etc/portal-static-assets-dev/database-url"
socket_path="$run_dir/event.sock"
api_pid="$run_dir/api.pid"
api_supervisor_pid="$run_dir/api-supervisor.pid"
core_pid="$run_dir/event-core.pid"
core_supervisor_pid="$run_dir/event-core-supervisor.pid"

ensure_directories() {
  install -d -o personalportal -g personalportal -m 0750 "$run_dir"
  install -d -o root -g wheel -m 0755 "$bin_dir"
  install -d -o root -g wheel -m 0750 "$log_dir"
  install -d -o personalportal -g personalportal -m 0750 "$storage_dir" "$upload_dir" "$resource_dir"
  if [ ! -s "$database_url_file" ]; then
    echo "Development database URL file is missing: $database_url_file" >&2
    return 1
  fi
  if [ ! -x "$bin_dir/api" ] || [ ! -x "$bin_dir/event-core" ]; then
    echo "Development API/event-core binaries are missing from $bin_dir" >&2
    return 1
  fi
}

pid_is_running() {
  pid_file="$1"
  [ -s "$pid_file" ] || return 1
  pid=$(cat "$pid_file")
  case "$pid" in *[!0-9]*|'') return 1 ;; esac
  kill -0 "$pid" 2>/dev/null
}

start_core() {
  if pid_is_running "$core_pid"; then
    command_line=$(ps -p "$(cat "$core_pid")" -o command= || true)
    case "$command_line" in
      "$bin_dir/event-core"*)
        if [ -S "$socket_path" ]; then return 0; fi
        echo "Development event-core process is running without its socket." >&2
        return 1
        ;;
      *) echo "Refusing to reuse unexpected process in $core_pid: $command_line" >&2; return 1 ;;
    esac
  fi
  rm -f "$core_pid" "$core_supervisor_pid"
  daemon -p "$core_pid" -P "$core_supervisor_pid" -u personalportal \
    -o "$log_dir/event-core.log" -M 0640 \
    "$bin_dir/event-core" "$socket_path"
  attempt=0
  while [ "$attempt" -lt 10 ]; do
    if [ -S "$socket_path" ] && pid_is_running "$core_pid"; then return 0; fi
    attempt=$((attempt + 1))
    sleep 1
  done
  echo "Development event core did not create its socket." >&2
  return 1
}

start_api() {
  if pid_is_running "$api_pid"; then
    api_process=$(cat "$api_pid")
    command_line=$(ps -p "$api_process" -o command= || true)
    case "$command_line" in
      "$bin_dir/api")
        if procstat -e "$api_process" | grep -q 'PERSONAL_PORTAL_REGISTRATION_ENABLED=0' &&
           procstat -e "$api_process" | grep -q "PERSONAL_PORTAL_UPLOAD_DIR=$upload_dir" &&
           procstat -e "$api_process" | grep -q "PERSONAL_PORTAL_RESOURCE_DIR=$resource_dir"; then
          return 0
        fi
        echo "Development API is running with unexpected registration/storage settings; use restart." >&2
        return 1
        ;;
      *) echo "Refusing to reuse unexpected process in $api_pid: $command_line" >&2; return 1 ;;
    esac
  fi
  rm -f "$api_pid" "$api_supervisor_pid"
  daemon -p "$api_pid" -P "$api_supervisor_pid" -u personalportal \
    -o "$log_dir/api.log" -M 0640 \
    /usr/bin/env \
      HOME="$storage_dir" USER=personalportal LOGNAME=personalportal \
      PATH=/sbin:/bin:/usr/sbin:/usr/bin:/usr/local/sbin:/usr/local/bin \
      PERSONAL_PORTAL_HTTP_ADDR=127.0.0.1:8082 \
      PERSONAL_PORTAL_EVENT_SOCKET="$socket_path" \
      PERSONAL_PORTAL_DATABASE_URL_FILE="$database_url_file" \
      PERSONAL_PORTAL_UPLOAD_DIR="$upload_dir" \
      PERSONAL_PORTAL_RESOURCE_DIR="$resource_dir" \
      PERSONAL_PORTAL_EVENT_WORKERS=2 \
      PERSONAL_PORTAL_REGISTRATION_ENABLED=0 \
      "$bin_dir/api"
}

wait_for_url() {
  url="$1"
  pattern="$2"
  attempt=0
  while [ "$attempt" -lt 30 ]; do
    if body=$(curl -fsS --max-time 3 "$url" 2>/dev/null) && printf '%s' "$body" | grep -q "$pattern"; then
      return 0
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
  echo "Development readiness timed out: $url" >&2
  tail -n 80 "$log_dir/api.log" "$log_dir/event-core.log" >&2 || true
  return 1
}

start_all() {
  ensure_directories
  start_core
  start_api
  wait_for_url http://127.0.0.1:8082/healthz '"status":"ok'
  wait_for_url http://127.0.0.1:8082/dbz '"status":"ok'
  wait_for_url http://127.0.0.1:8082/readyz '"status":"ready'
  echo "Development API and C event core are ready. Registration remains disabled."
}

stop_one() {
  child_file="$1"
  supervisor_file="$2"
  expected_prefix="$3"
  if pid_is_running "$child_file"; then
    child=$(cat "$child_file")
    command_line=$(ps -p "$child" -o command= || true)
    case "$command_line" in
      "$expected_prefix"*) kill -TERM "$child" ;;
      *) echo "Refusing to stop unexpected process $child: $command_line" >&2; return 1 ;;
    esac
    attempt=0
    while kill -0 "$child" 2>/dev/null && [ "$attempt" -lt 20 ]; do
      attempt=$((attempt + 1))
      sleep 1
    done
    if kill -0 "$child" 2>/dev/null; then
      echo "Development process $child did not stop gracefully; no SIGKILL was sent." >&2
      return 1
    fi
  fi
  if pid_is_running "$supervisor_file"; then
    supervisor=$(cat "$supervisor_file")
    supervisor_command=$(ps -p "$supervisor" -o command= || true)
    case "$supervisor_command" in
      "daemon: "*) kill -TERM "$supervisor" ;;
      *) echo "Refusing to stop unexpected supervisor $supervisor: $supervisor_command" >&2; return 1 ;;
    esac
    attempt=0
    while kill -0 "$supervisor" 2>/dev/null && [ "$attempt" -lt 5 ]; do
      attempt=$((attempt + 1))
      sleep 1
    done
    if kill -0 "$supervisor" 2>/dev/null; then
      echo "Development supervisor $supervisor did not exit after child shutdown." >&2
      return 1
    fi
  fi
  rm -f "$child_file" "$supervisor_file"
}

stop_all() {
  stop_one "$api_pid" "$api_supervisor_pid" "$bin_dir/api"
  stop_one "$core_pid" "$core_supervisor_pid" "$bin_dir/event-core"
  echo "Development API and C event core stopped."
}

show_status() {
  for pid_file in "$api_pid" "$core_pid"; do
    if pid_is_running "$pid_file"; then
      pid=$(cat "$pid_file")
      ps -p "$pid" -o pid,ppid,user,group,command
    else
      echo "STOPPED: $pid_file"
    fi
  done
}

case "$command_name" in
  start) start_all ;;
  stop) stop_all ;;
  restart) stop_all; start_all ;;
  status) show_status ;;
  *) echo "Usage: $0 {start|stop|restart|status}" >&2; exit 2 ;;
esac
