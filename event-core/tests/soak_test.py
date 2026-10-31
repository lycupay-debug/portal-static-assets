#!/usr/bin/env python3
"""Controlled multi-subscriber soak test for the C event core.

This is an opt-in operational test, not part of the short default CI suite.
PUBLISH uses one connection per request because non-subscriber clients are
intentionally closed after their single response.
"""

from __future__ import annotations

import argparse
import os
import re
import signal
import socket
import subprocess
import tempfile
import threading
import time
import uuid


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", help="path to the event-core executable")
    parser.add_argument("--duration", type=int, default=300, help="soak duration in seconds (default: 300)")
    parser.add_argument("--subscribers", type=int, default=16, help="persistent subscriber connections (default: 16)")
    parser.add_argument("--rate", type=float, default=10.0, help="publish rate per second (default: 10)")
    args = parser.parse_args()
    if args.duration < 1 or not 1 <= args.subscribers <= 48 or args.rate <= 0:
        parser.error("duration and rate must be positive; subscribers must be between 1 and 48")
    return args


def main() -> int:
    args = parse_args()
    binary = os.path.abspath(args.binary)
    event_type = "portal.soak.test"
    stop = threading.Event()
    lock = threading.Lock()
    errors: list[str] = []
    sockets: list[socket.socket] = []
    ack_counts = [0] * args.subscribers
    rss_samples: list[int] = []
    published = 0
    elapsed = 0.0
    process: subprocess.Popen[bytes] | None = None
    log_handle = None
    failure: str | None = None

    with tempfile.TemporaryDirectory(prefix="portal-event-core-soak-") as directory:
        socket_path = os.path.join(directory, "event.sock")
        log_path = os.path.join(directory, "event-core.log")
        log_handle = open(log_path, "w", encoding="utf-8")
        try:
            process = subprocess.Popen(
                [binary, socket_path],
                stdout=log_handle,
                stderr=subprocess.STDOUT,
            )

            ready_by = time.monotonic() + 5
            while time.monotonic() < ready_by:
                if process.poll() is not None:
                    raise RuntimeError(f"event core exited before readiness: {process.returncode}")
                if os.path.exists(socket_path):
                    try:
                        with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as probe:
                            probe.settimeout(0.5)
                            probe.connect(socket_path)
                            probe.sendall(b"v2 probe STATUS\n")
                            if probe.recv(128) == b"v2 probe OK RUNNING\n":
                                break
                    except OSError:
                        pass
                time.sleep(0.02)
            else:
                raise RuntimeError("event core readiness timeout")

            def subscriber_worker(index: int) -> None:
                sock: socket.socket | None = None
                stream = None
                try:
                    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
                    sock.settimeout(10)
                    sock.connect(socket_path)
                    stream = sock.makefile("rwb", buffering=0)
                    request_id = f"sub{index}"
                    stream.write(f"v2 {request_id} SUBSCRIBE\n".encode())
                    if stream.readline() != f"v2 {request_id} OK SUBSCRIBED\n".encode():
                        raise RuntimeError(f"subscriber {index} failed to subscribe")
                    with lock:
                        sockets.append(sock)

                    sequence = 0
                    while not stop.is_set():
                        line = stream.readline()
                        if not line:
                            break
                        fields = line.decode("ascii").strip().split()
                        if len(fields) != 5 or fields[2] != "EVENT":
                            raise RuntimeError(f"subscriber {index} received malformed push: {line!r}")
                        sequence += 1
                        ack_id = f"a{index}-{sequence}"
                        stream.write(f"v2 {ack_id} ACK {fields[3]} {fields[4]}\n".encode())
                        response = stream.readline()
                        if response != f"v2 {ack_id} OK ACKNOWLEDGED\n".encode():
                            raise RuntimeError(f"subscriber {index} received bad ACK response: {response!r}")
                        with lock:
                            ack_counts[index] += 1
                except (OSError, TimeoutError) as exc:
                    if not stop.is_set():
                        with lock:
                            errors.append(f"subscriber {index} socket error: {exc}")
                except Exception as exc:
                    with lock:
                        errors.append(f"subscriber {index}: {exc}")
                finally:
                    if stream is not None:
                        try:
                            stream.close()
                        except Exception:
                            pass
                    if sock is not None:
                        try:
                            sock.close()
                        except OSError:
                            pass

            threads = [
                threading.Thread(target=subscriber_worker, args=(index,), daemon=True)
                for index in range(args.subscribers)
            ]
            for thread in threads:
                thread.start()

            connected_by = time.monotonic() + 5
            while time.monotonic() < connected_by:
                with lock:
                    connected = len(sockets)
                    thread_errors = list(errors)
                if thread_errors:
                    raise RuntimeError("; ".join(thread_errors))
                if connected == args.subscribers:
                    break
                time.sleep(0.02)
            else:
                raise RuntimeError(f"only {len(sockets)} of {args.subscribers} subscribers connected")

            start = time.monotonic()
            end = start + args.duration
            next_tick = start
            while time.monotonic() < end:
                published += 1
                event_id = str(uuid.uuid4())
                # The C core closes non-subscriber clients after one response.
                with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as publisher:
                    publisher.settimeout(5)
                    publisher.connect(socket_path)
                    stream = publisher.makefile("rwb", buffering=0)
                    request_id = f"p{published}"
                    stream.write(f"v2 {request_id} PUBLISH {event_id} {event_type}\n".encode())
                    response = stream.readline()
                    stream.close()
                if response != f"v2 p{published} OK PUBLISHED\n".encode():
                    raise RuntimeError(f"publish {published} returned {response!r}")
                if process.poll() is not None:
                    raise RuntimeError(f"event core exited unexpectedly: {process.returncode}")
                if published % 100 == 0:
                    rss = subprocess.check_output(
                        ["ps", "-o", "rss=", "-p", str(process.pid)],
                        text=True,
                    ).strip()
                    if rss:
                        rss_samples.append(int(rss))
                with lock:
                    if errors:
                        raise RuntimeError("; ".join(errors))
                next_tick += 1.0 / args.rate
                delay = next_tick - time.monotonic()
                if delay > 0:
                    time.sleep(delay)
            elapsed = time.monotonic() - start

            drain_by = time.monotonic() + 10
            while time.monotonic() < drain_by:
                with lock:
                    drained = all(count == published for count in ack_counts)
                    thread_errors = list(errors)
                if thread_errors:
                    raise RuntimeError("; ".join(thread_errors))
                if drained:
                    break
                time.sleep(0.05)

            with lock:
                final_counts = list(ack_counts)
            if any(count != published for count in final_counts):
                raise RuntimeError(f"subscriber ACK counts incomplete: {final_counts}, published={published}")

        except Exception as exc:
            failure = str(exc)
        finally:
            stop.set()
            with lock:
                active_sockets = list(sockets)
            for sock in active_sockets:
                try:
                    sock.shutdown(socket.SHUT_RDWR)
                except OSError:
                    pass
                try:
                    sock.close()
                except OSError:
                    pass
            if process is not None and process.poll() is None:
                process.send_signal(signal.SIGTERM)
                try:
                    process.wait(timeout=4)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
            if log_handle is not None:
                log_handle.close()

        log_text = open(log_path, encoding="utf-8", errors="replace").read()
        overflows = log_text.count("event_core_push_queue_overflow")
        ack_timeouts = log_text.count("event_core_ack_timeout")
        queue_depths = [int(match.group(1)) for match in re.finditer(r"queue_depth=(\d+)", log_text)]
        max_queue_depth = max(queue_depths, default=0)
        process_exit = process.returncode if process is not None else None
        with lock:
            final_counts = list(ack_counts)
            thread_errors = list(errors)

        print(
            f"duration_seconds={elapsed:.1f} subscribers={args.subscribers} "
            f"target_rate={args.rate}/s published={published}"
        )
        print(f"ack_counts={final_counts}")
        print(
            f"rss_kib_min={min(rss_samples) if rss_samples else 'n/a'} "
            f"rss_kib_max={max(rss_samples) if rss_samples else 'n/a'} "
            f"samples={len(rss_samples)}"
        )
        print(
            f"max_logged_queue_depth={max_queue_depth} queue_overflow_logs={overflows} "
            f"ack_timeout_logs={ack_timeouts} process_exit={process_exit} "
            f"thread_errors={thread_errors}"
        )

        if failure or process_exit != 0 or thread_errors or overflows or ack_timeouts:
            print(f"FAIL: {failure or 'event-core soak invariants failed'}")
            return 1
        if any(count != published for count in final_counts):
            print("FAIL: at least one subscriber did not ACK every published event")
            return 1

    print("PASS: event-core soak completed with no lost ACKs, queue overflow, ACK timeout, or process failure")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
