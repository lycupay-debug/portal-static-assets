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

PYTHON=${PYTHON:-python3.14}
if ! command -v "$PYTHON" >/dev/null 2>&1; then PYTHON=python3; fi
"$PYTHON" - "$SOCKET" "$LOG" <<'PY'
import socket
import sys
import threading

path = sys.argv[1]
log_path = sys.argv[2]

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
    (b"v2 ping1 PING\n", "v2 ping1 OK PONG\n"),
    (b"v2 stat1 STATUS\n", "v2 stat1 OK RUNNING\n"),
    (b"v2 evt1 EVENT 018f2f5e-7a2b-7c4d-8e9f-0123456789ab content.published\n",
     "v2 evt1 OK ACCEPTED\n"),
    (b"v2 evbad EVENT not-a-uuid content.published\n",
     "v2 0 ERROR BAD_REQUEST\n"),
    (b"v2 evtype EVENT 018f2f5e-7a2b-7c4d-8e9f-0123456789ab Content.Published\n",
     "v2 0 ERROR BAD_REQUEST\n"),
    (b"v2 evextra EVENT 018f2f5e-7a2b-7c4d-8e9f-0123456789ab content.published extra\n",
     "v2 0 ERROR BAD_REQUEST\n"),
    (b"v2 unk1 NOPE\n", "v2 unk1 ERROR UNKNOWN_COMMAND\n"),
    (b"v1 ping1 PING\n", "v2 0 ERROR BAD_REQUEST\n"),
    (b"v2 bad.id PING\n", "v2 0 ERROR BAD_REQUEST\n"),
    (b"v2 nul PING\x00\n", "v2 0 ERROR BAD_REQUEST\n"),
]
for payload, expected in checks:
    actual = request(payload)
    if actual != expected:
        raise SystemExit(f"FAIL: {payload!r}: expected {expected!r}, got {actual!r}")

oversized = request(b"x" * 300)
if oversized != "v2 0 ERROR REQUEST_TOO_LARGE\n":
    raise SystemExit(f"FAIL: oversized request response was {oversized!r}")

subscriber = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
subscriber.settimeout(3)
subscriber.connect(path)
subscriber.sendall(b"v2 sub1 SUBSCRIBE\n")
ack = bytearray()
while not ack.endswith(b"\n"):
    ack.extend(subscriber.recv(256))
if ack.decode("ascii") != "v2 sub1 OK SUBSCRIBED\n":
    raise SystemExit(f"FAIL: subscribe acknowledgement was {ack!r}")

published_id = "018f2f5e-7a2b-7c4d-8e9f-0123456789ab"
publish_response = request(
    f"v2 pub1 PUBLISH {published_id} portal.content.validate\n".encode("ascii"))
if publish_response != "v2 pub1 OK PUBLISHED\n":
    raise SystemExit(f"FAIL: publish response was {publish_response!r}")
notification = bytearray()
while not notification.endswith(b"\n"):
    notification.extend(subscriber.recv(256))
expected_notification = f"v2 push1 EVENT {published_id} portal.content.validate\n"
if notification.decode("ascii") != expected_notification:
    raise SystemExit(f"FAIL: subscriber notification was {notification!r}")
second_id = "018f2f5e-7a2b-7c4d-8e9f-1123456789ab"
queued_response = request(f"v2 pub2 PUBLISH {second_id} portal.content.validate\n".encode("ascii"))
if queued_response != "v2 pub2 OK PUBLISHED\n":
    raise SystemExit(f"FAIL: second publish response was {queued_response!r}")
subscriber.sendall(f"v2 ack1 ACK {published_id} portal.content.validate\n".encode("ascii"))
ack_response = bytearray()
while b"\n" not in ack_response:
    ack_response.extend(subscriber.recv(256))
ack_line, queued_bytes = ack_response.split(b"\n", 1)
if ack_line.decode("ascii") != "v2 ack1 OK ACKNOWLEDGED":
    raise SystemExit(f"FAIL: subscriber ACK response was {ack_line!r}")
queued_notification = bytearray(queued_bytes)
while not queued_notification.endswith(b"\n"):
    queued_notification.extend(subscriber.recv(256))
expected_queued = f"v2 push2 EVENT {second_id} portal.content.validate\n"
if queued_notification.decode("ascii") != expected_queued:
    raise SystemExit(f"FAIL: queued notification was {queued_notification!r}")
subscriber.sendall(f"v2 ack2 ACK {second_id} portal.content.validate\n".encode("ascii"))
second_ack = bytearray()
while not second_ack.endswith(b"\n"):
    second_ack.extend(subscriber.recv(256))
if second_ack.decode("ascii") != "v2 ack2 OK ACKNOWLEDGED\n":
    raise SystemExit(f"FAIL: second ACK response was {second_ack!r}")
subscriber.sendall(f"v2 ackbad ACK {second_id} portal.content.validate\n".encode("ascii"))
bad_ack = bytearray()
while not bad_ack.endswith(b"\n"):
    bad_ack.extend(subscriber.recv(256))
if bad_ack.decode("ascii") != "v2 ackbad ERROR BAD_ACK\n":
    raise SystemExit(f"FAIL: duplicate/mismatched ACK response was {bad_ack!r}")
# Fill the bounded metadata queue while a push awaits ACK; overflow must be observable.
for index in range(34):
    event_id = f"018f2f5e-7a2b-7c4d-8e9f-{index + 100:012x}"
    response = request(f"v2 overflow{index} PUBLISH {event_id} portal.content.validate\n".encode("ascii"))
    if response != f"v2 overflow{index} OK PUBLISHED\n":
        raise SystemExit(f"FAIL: queue overflow publish response was {response!r}")
subscriber.close()

# A client that never completes its request must not occupy a slot forever.
idle = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
idle.settimeout(8)
idle.connect(path)
if idle.recv(1) != b"":
    raise SystemExit("FAIL: idle client received unexpected data")
idle.close()
with open(log_path, encoding="utf-8") as log_file:
    if "event_core_client_timeout" not in log_file.read():
        raise SystemExit("FAIL: idle-client timeout log was not flushed while the core was running")

# A subscriber that never ACKs a push must be disconnected after the bounded ACK timeout.
slow = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
slow.settimeout(8)
slow.connect(path)
slow.sendall(b"v2 slow1 SUBSCRIBE\n")
slow_response = bytearray()
while not slow_response.endswith(b"\n"):
    slow_response.extend(slow.recv(256))
if slow_response.decode("ascii") != "v2 slow1 OK SUBSCRIBED\n":
    raise SystemExit(f"FAIL: slow subscriber handshake was {slow_response!r}")
slow_event_id = "018f2f5e-7a2b-7c4d-8e9f-2123456789ab"
slow_publish = request(f"v2 slowpub PUBLISH {slow_event_id} portal.content.validate\n".encode("ascii"))
if slow_publish != "v2 slowpub OK PUBLISHED\n":
    raise SystemExit(f"FAIL: slow publish response was {slow_publish!r}")
slow_push = bytearray()
while not slow_push.endswith(b"\n"):
    slow_push.extend(slow.recv(256))
if f"{slow_event_id} portal.content.validate".encode("ascii") not in slow_push:
    raise SystemExit(f"FAIL: slow subscriber push was {slow_push!r}")
if slow.recv(1) != b"":
    raise SystemExit("FAIL: unacknowledged subscriber was not closed")
slow.close()
with open(log_path, encoding="utf-8") as log_file:
    if "event_core_ack_timeout" not in log_file.read():
        raise SystemExit("FAIL: ACK-timeout log was not flushed while the core was running")

errors = []
def parallel_check(index):
    try:
        actual = request(f"v2 parallel{index} PING\n".encode("ascii"))
        expected = f"v2 parallel{index} OK PONG\n"
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

print("PASS: PING, STATUS, EVENT validation, SUBSCRIBE/PUBLISH push, ACK-gated FIFO queue, duplicate ACK handling, overflow logging, unknown command, protocol validation, request limit, 24 concurrent clients")
PY

kill -TERM "$PID"
wait "$PID"
PID=""
grep -q "event-core stopped" "$LOG"
grep -q "event_core_push_queue_overflow" "$LOG"
grep -q "event_core_ack_timeout" "$LOG"
grep -q "event_core_client_timeout" "$LOG"
echo "PASS: SIGTERM shutdown, bounded queue, idle-client expiry, and ACK timeout observability"
