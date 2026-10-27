#!/usr/local/bin/python3.14
"""Project sanitized KERNO/GSM audit events into the portal SSH log feed."""
import json
import os
import tempfile
from datetime import datetime

SOURCE = "/var/log/kerno-audit.log"
AUDIT = "/var/log/host-mcp-admin-audit.log"
STATE = "/var/db/go-ssh-mcp-service-log.cursor"
ALLOWED = {
    "shell.exec", "file.read", "file.write", "file.replace", "file.move",
    "file.delete", "directory.list", "system.info", "service.status",
    "logs.tail", "database.query", "execute_parallel",
}
MAX_LINE = 65536


def parse_time(value):
    if not isinstance(value, str):
        return None
    try:
        result = datetime.fromisoformat(value.replace("Z", "+00:00"))
        if result.tzinfo is None:
            return None
        return result.astimezone().isoformat(timespec="seconds")
    except (ValueError, OverflowError):
        return None


def save_state(state):
    directory = os.path.dirname(STATE)
    fd, tmp = tempfile.mkstemp(prefix=".go-ssh-log-cursor-", dir=directory)
    try:
        with os.fdopen(fd, "w", encoding="ascii") as f:
            json.dump(state, f, separators=(",", ":"))
            f.write("\n")
            f.flush()
            os.fsync(f.fileno())
        os.chmod(tmp, 0o600)
        os.replace(tmp, STATE)
    finally:
        try:
            os.unlink(tmp)
        except FileNotFoundError:
            pass


def project(obj):
    if not isinstance(obj, dict):
        return None
    operation = obj.get("operation")
    if not isinstance(operation, str) or operation not in ALLOWED:
        return None
    timestamp = parse_time(obj.get("timestamp"))
    if timestamp is None:
        return None
    returncode = obj.get("returncode")
    has_failed_returncode = isinstance(returncode, int) and not isinstance(returncode, bool) and returncode != 0
    if obj.get("timed_out") is True:
        outcome, action = "timeout", "tool.operation.failed"
    elif obj.get("success") is False or has_failed_returncode:
        outcome, action = "failed", "tool.operation.failed"
    else:
        outcome, action = "success", "tool.operation.completed"
    details = {"tool_source": "Go SSH MCP", "operation": operation}
    duration = obj.get("duration_ms")
    if isinstance(duration, int) and not isinstance(duration, bool) and 0 <= duration <= 86400000:
        details["duration_ms"] = duration
    if isinstance(obj.get("returncode"), int) and not isinstance(obj.get("returncode"), bool):
        details["returncode"] = obj["returncode"]
    if obj.get("timed_out") is True:
        details["timed_out"] = True
    return {"time": timestamp, "action": action, "outcome": outcome, "uid": 0, "details": details}


def main():
    try:
        st = os.stat(SOURCE)
    except OSError:
        return 0
    try:
        with open(STATE, "r", encoding="ascii") as f:
            state = json.load(f)
        offset, old_inode = int(state.get("offset", 0)), int(state.get("inode", 0))
    except (OSError, ValueError, TypeError, json.JSONDecodeError):
        offset, old_inode = 0, 0
    if old_inode != st.st_ino or offset < 0 or offset > st.st_size:
        offset = 0
    os.makedirs(os.path.dirname(AUDIT), exist_ok=True)
    if not os.path.exists(AUDIT):
        fd = os.open(AUDIT, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o640)
        os.close(fd)
    os.chown(AUDIT, 0, __import__("grp").getgrnam("personalportal").gr_gid)
    os.chmod(AUDIT, 0o640)
    with open(SOURCE, "rb") as src, open(AUDIT, "a", encoding="utf-8") as dest:
        src.seek(offset)
        while True:
            start = src.tell()
            raw = src.readline(MAX_LINE + 1)
            if not raw:
                break
            if not raw.endswith(b"\n"):
                src.seek(start)
                break
            end = src.tell()
            if len(raw) <= MAX_LINE:
                try:
                    record = project(json.loads(raw))
                except (ValueError, UnicodeDecodeError):
                    record = None
                if record is not None:
                    dest.write(json.dumps(record, ensure_ascii=False, separators=(",", ":")) + "\n")
                    dest.flush()
                    os.fsync(dest.fileno())
            offset = end
            save_state({"inode": st.st_ino, "offset": offset})
    if not os.path.exists(STATE):
        save_state({"inode": st.st_ino, "offset": offset})
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
