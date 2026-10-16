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
    (b"v1 evt1 EVENT 018f2f5e-7a2b-7c4d-8e9f-0123456789ab content.published\n",
     "v1 evt1 OK ACCEPTED\n"),
    (b"v1 evbad EVENT not-a-uuid content.published\n",
     "v1 0 ERROR BAD_REQUEST\n"),
    (b"v1 evtype EVENT 018f2f5e-7a2b-7c4d-8e9f-0123456789ab Content.Published\n",
     "v1 0 ERROR BAD_REQUEST\n"),
    (b"v1 evextra EVENT 018f2f5e-7a2b-7c4d-8e9f-0123456789ab content.published extra\n",
     "v1 0 ERROR BAD_REQUEST\n"),
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

subscriber = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
subscriber.settimeout(3)
subscriber.connect(path)
subscriber.sendall(b"v1 sub1 SUBSCRIBE\n")
ack = bytearray()
while not ack.endswith(b"\n"):
    ack.extend(subscriber.recv(256))
if ack.decode("ascii") != "v1 sub1 OK SUBSCRIBED\n":
    raise SystemExit(f"FAIL: subscribe acknowledgement was {ack!r}")

published_id = "018f2f5e-7a2b-7c4d-8e9f-0123456789ab"
publish_response = request(
    f"v1 pub1 PUBLISH {published_id} portal.content.validate\n".encode("ascii"))
if publish_response != "v1 pub1 OK PUBLISHED\n":
    raise SystemExit(f"FAIL: publish response was {publish_response!r}")
notification = bytearray()
while not notification.endswith(b"\n"):
    notification.extend(subscriber.recv(256))
expected_notification = f"v1 push1 EVENT {published_id} portal.content.validate\n"
if notification.decode("ascii") != expected_notification:
    raise SystemExit(f"FAIL: subscriber notification was {notification!r}")
subscriber.close()

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

print("PASS: PING, STATUS, EVENT validation, SUBSCRIBE/PUBLISH push, unknown command, protocol validation, request limit, 24 concurrent clients")
PY

kill -TERM "$PID"
wait "$PID"
PID=""
grep -q "event-core stopped" "$LOG"
echo "PASS: SIGTERM shutdown"
