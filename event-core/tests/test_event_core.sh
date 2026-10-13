#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
SOCKET="/var/tmp/personal-portal-event-core-test-$$.sock"
LOG="/var/tmp/personal-portal-event-core-test-$$.log"
PID=""

cleanup() {
    if [ -n "$PID" ]; then
        kill -TERM "$PID" 2>/dev/null || true
        wait "$PID" 2>/dev/null || true
    fi
    rm -f "$SOCKET" "$LOG"
}
trap cleanup EXIT INT TERM

"$ROOT/event-core" "$SOCKET" >"$LOG" 2>&1 &
PID=$!

i=0
while [ ! -S "$SOCKET" ]; do
    i=$((i + 1))
    if [ "$i" -ge 50 ]; then
        cat "$LOG"
        echo "FAIL: event socket did not appear" >&2
        exit 1
    fi
    sleep 0.1
done

python3.14 - "$SOCKET" <<'PY'
import socket
import sys
import threading

path = sys.argv[1]

def request(data):
    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as client:
        client.settimeout(3)
        client.connect(path)
        client.sendall(data)
        response = bytearray()
        while not response.endswith(b"\n"):
            chunk = client.recv(256)
            if not chunk:
                break
            response.extend(chunk)
        return response.decode("ascii")

checks = [
    (b"v1 ping1 PING\n", "v1 ping1 OK PONG\n"),
    (b"v1 stat1 STATUS\n", "v1 stat1 OK RUNNING\n"),
    (b"v1 unk1 NOPE\n", "v1 unk1 ERROR UNKNOWN_COMMAND\n"),
    (b"v2 ping1 PING\n", "v1 0 ERROR BAD_REQUEST\n"),
    (b"v1 bad.id PING\n", "v1 0 ERROR BAD_REQUEST\n"),
    (b"v1 nul PING\x00\n", "v1 0 ERROR BAD_REQUEST\n"),
]
for payload, expected in checks:
    actual = request(payload)
    if actual != expected:
        raise SystemExit(f"FAIL: {payload!r}: expected {expected!r}, got {actual!r}")

oversized = request(b"x" * 300)
if oversized != "v1 0 ERROR REQUEST_TOO_LARGE\n":
    raise SystemExit(f"FAIL: oversized request response was {oversized!r}")

errors = []
def parallel_check(index):
    try:
        actual = request(f"v1 parallel{index} PING\n".encode("ascii"))
        expected = f"v1 parallel{index} OK PONG\n"
        if actual != expected:
            errors.append((index, actual))
    except Exception as exc:
        errors.append((index, repr(exc)))

threads = [threading.Thread(target=parallel_check, args=(i,)) for i in range(24)]
for thread in threads:
    thread.start()
for thread in threads:
    thread.join()
if errors:
    raise SystemExit(f"FAIL: concurrent requests: {errors!r}")

print("PASS: PING, STATUS, unknown command, protocol validation, request limit, 24 concurrent clients")
PY

kill -TERM "$PID"
wait "$PID"
PID=""
grep -q "event-core stopped" "$LOG"
echo "PASS: SIGTERM shutdown"
