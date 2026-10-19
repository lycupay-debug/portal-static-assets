#!/usr/local/bin/python3.14
"""Sanitize Go SSH MCP JSON operation logs into the portal SSH audit feed."""
import json
import os
import stat
import tempfile
from datetime import datetime, timezone

SOURCE = "/var/log/go-ssh-mcp.log"
AUDIT = "/var/log/host-mcp-admin-audit.log"
STATE = "/var/db/go-ssh-mcp-service-log.cursor"
ALLOWED = {
    "shell.exec", "file.read", "file.write", "file.replace", "file.move",
    "file.delete", "directory.list", "system.info", "service.status", "logs.tail",
}
MAX_LINE = 65536


def parse_time(value):
    if not isinstance(value, str):
        return None
    try:
        return datetime.fromisoformat(value.replace("Z", "+00:00")).astimezone(timezone.utc)
    except (ValueError, OverflowError):
        return None


def latest_audit_time():
    try:
        with open(AUDIT, "rb") as f:
            f.seek(0, os.SEEK_END)
            size = f.tell()
            f.seek(max(0, size - 65536))
            lines = f.read().splitlines()
        for raw in reversed(lines):
            try:
                value = json.loads(raw)
                t = parse_time(value.get("time"))
                if t:
                    return t
            except (ValueError, UnicodeDecodeError, AttributeError):
                continue
    except OSError:
        pass
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


def safe_record(obj):
    if not isinstance(obj, dict):
        return None
    operation = obj.get("operation")
    if operation not in ALLOWED:
        return None
    timestamp = obj.get("time")
    parsed = parse_time(timestamp)
    if parsed is None:
        return None
    level = str(obj.get("level", "INFO")).upper()
    msg = str(obj.get("msg", "")).lower()
    if "timeout" in msg or "timed out" in msg:
        outcome, event = "timeout", "tool.operation.failed"
    elif level in ("ERROR", "CRITICAL"):
        outcome, event = "failed", "tool.operation.failed"
    elif "completed" in msg:
        outcome, event = "success", "tool.operation.completed"
    else:
        outcome, event = "info", "tool.operation"
    details = {"tool_source": "Go SSH MCP", "operation": operation}
    duration = obj.get("duration_ms")
    if isinstance(duration, (int, float)) and not isinstance(duration, bool) and 0 <= duration <= 86400000:
        details["duration_ms"] = int(duration)
    return {
        "time": parsed.strftime("%Y-%m-%dT%H:%M:%SZ"),
        "action": event,
        "outcome": outcome,
        "uid": 0,
        "details": details,
    }


def main():
    try:
        st = os.stat(SOURCE)
    except OSError:
        return 0
    try:
        with open(STATE, "r", encoding="ascii") as f:
            state = json.load(f)
        offset = int(state.get("offset", 0))
        old_inode = int(state.get("inode", 0))
    except (OSError, ValueError, TypeError, json.JSONDecodeError):
        offset, old_inode = 0, 0
    rotated = old_inode not in (0, st.st_ino) or offset > st.st_size
    if rotated:
        offset = 0
    first_run = old_inode == 0
    cutoff = latest_audit_time() if first_run else None
    if offset < 0:
        offset = 0
    with open(SOURCE, "rb") as src, open(AUDIT, "a", encoding="utf-8") as audit:
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
            if len(raw) > MAX_LINE:
                offset = end
                continue
            try:
                obj = json.loads(raw)
                record = safe_record(obj)
            except (ValueError, UnicodeDecodeError):
                record = None
            if record:
                parsed = parse_time(record["time"])
                if cutoff is None or parsed > cutoff:
                    audit.write(json.dumps(record, ensure_ascii=False, separators=(",", ":")) + "\n")
                    audit.flush()
                    os.fsync(audit.fileno())
            offset = end
            save_state({"inode": st.st_ino, "offset": offset})
    if not os.path.exists(STATE):
        save_state({"inode": st.st_ino, "offset": offset})
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
