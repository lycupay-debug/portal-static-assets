#!/usr/bin/env python3
"""Exercise the real FreeBSD/macOS kqueue event loop over a Unix socket."""

import os
import signal
import socket
import subprocess
import sys
import tempfile
import time


STATUS_REQUEST = b"v2 probe1 STATUS\n"
STATUS_RESPONSE = b"v2 probe1 OK RUNNING\n"
EVENT_ID = b"550e8400-e29b-41d4-a716-446655440000"
EVENT_TYPE = b"portal.test.live"


def wait_ready(binary, socket_path):
    process = subprocess.Popen(
        [binary, socket_path],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.PIPE,
        text=True,
    )
    deadline = time.monotonic() + 5
    while time.monotonic() < deadline:
        if process.poll() is not None:
            stderr = process.stderr.read() if process.stderr else ""
            raise AssertionError(f"event core exited before readiness: {stderr}")
        if os.path.exists(socket_path):
            try:
                with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as client:
                    client.settimeout(0.5)
                    client.connect(socket_path)
                    client.sendall(STATUS_REQUEST)
                    if client.recv(256) == STATUS_RESPONSE:
                        return process
            except (OSError, TimeoutError):
                pass
        time.sleep(0.025)
    process.kill()
    process.wait(timeout=2)
    raise AssertionError("event core did not become ready within five seconds")


def stop_core(process, socket_path):
    if process.poll() is None:
        process.send_signal(signal.SIGTERM)
        try:
            result = process.wait(timeout=3)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=2)
            raise AssertionError("SIGTERM did not stop the kqueue event loop")
        if result != 0:
            stderr = process.stderr.read() if process.stderr else ""
            raise AssertionError(f"event core exited with {result}: {stderr}")
    if os.path.exists(socket_path):
        raise AssertionError("event socket remained after shutdown cleanup")


def exchange(socket_path, request, timeout=2):
    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as client:
        client.settimeout(timeout)
        client.connect(socket_path)
        client.sendall(request)
        return client.recv(512)


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: integration_test.py /path/to/event-core")
    binary = os.path.abspath(sys.argv[1])
    with tempfile.TemporaryDirectory(prefix="portal-event-core-test-") as directory:
        socket_path = os.path.join(directory, "event.sock")
        process = wait_ready(binary, socket_path)
        try:
            assert exchange(socket_path, STATUS_REQUEST) == STATUS_RESPONSE
            assert exchange(socket_path, b"v2 bad id PING\n") == b"v2 0 ERROR BAD_REQUEST\n"
            assert exchange(socket_path, b"v2 nul1 PING\x00\n") == b"v2 0 ERROR BAD_REQUEST\n"
            assert exchange(socket_path, b"x" * 300) == b"v2 0 ERROR REQUEST_TOO_LARGE\n"

            # Exercise a real subscriber push and matching ACK over kqueue.
            subscriber = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            publisher = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            subscriber.settimeout(2)
            publisher.settimeout(2)
            subscriber.connect(socket_path)
            subscriber.sendall(b"v2 sub1 SUBSCRIBE\n")
            assert subscriber.recv(256) == b"v2 sub1 OK SUBSCRIBED\n"
            publisher.connect(socket_path)
            publisher.sendall(b"v2 pub1 PUBLISH " + EVENT_ID + b" " + EVENT_TYPE + b"\n")
            assert publisher.recv(256) == b"v2 pub1 OK PUBLISHED\n"
            push = subscriber.recv(256)
            assert push.startswith(b"v2 push") and push.endswith(
                b"EVENT " + EVENT_ID + b" " + EVENT_TYPE + b"\n"
            ), push
            subscriber.sendall(b"v2 ack1 ACK " + EVENT_ID + b" " + EVENT_TYPE + b"\n")
            assert subscriber.recv(256) == b"v2 ack1 OK ACKNOWLEDGED\n"
            publisher.close()
            subscriber.close()

            # Hold all client slots; the next accepted connection must be closed.
            held = []
            for _ in range(64):
                client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
                client.settimeout(2)
                client.connect(socket_path)
                held.append(client)
            time.sleep(0.1)
            overflow = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            overflow.settimeout(2)
            overflow.connect(socket_path)
            assert overflow.recv(1) == b"", "65th client was not rejected at the configured cap"
            overflow.close()
            for client in held:
                client.close()

            # A client that never completes its request must be reclaimed.
            idle = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            idle.settimeout(7)
            idle.connect(socket_path)
            time.sleep(6)
            assert idle.recv(1) == b"", "idle client survived the request timeout"
            idle.close()

            # An unacknowledged push must also reclaim the subscriber slot.
            slow_subscriber = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            slow_publisher = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            slow_subscriber.settimeout(7)
            slow_publisher.settimeout(2)
            slow_subscriber.connect(socket_path)
            slow_subscriber.sendall(b"v2 sub2 SUBSCRIBE\n")
            assert slow_subscriber.recv(256) == b"v2 sub2 OK SUBSCRIBED\n"
            slow_publisher.connect(socket_path)
            slow_publisher.sendall(
                b"v2 pub2 PUBLISH 550e8400-e29b-41d4-a716-446655440001 portal.test.timeout\n"
            )
            assert slow_publisher.recv(256) == b"v2 pub2 OK PUBLISHED\n"
            assert slow_subscriber.recv(256).startswith(b"v2 push")
            time.sleep(6)
            assert slow_subscriber.recv(1) == b"", "subscriber survived the ACK timeout"
            slow_publisher.close()
            slow_subscriber.close()
        finally:
            stop_core(process, socket_path)

        # Restart on the same path to verify stale-socket cleanup and rebind.
        restarted = wait_ready(binary, socket_path)
        stop_core(restarted, socket_path)

    print("C event-core live kqueue integration tests passed")


if __name__ == "__main__":
    main()
