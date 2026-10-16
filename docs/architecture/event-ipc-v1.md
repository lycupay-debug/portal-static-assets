# Event Core IPC Contract v1

## Status

Version 1 is the first local connectivity contract between the C event core and future backend clients. It supports health/connectivity commands plus an EVENT metadata envelope. EVENT acceptance validates metadata only; it is not durable delivery or business processing.

## Transport

- Transport: Unix Domain Socket, `AF_UNIX` / `SOCK_STREAM`.
- Default socket path: `/var/run/personal-portal/event.sock`.
- Development/test runs may pass an alternate socket path as the C program's only argument.
- The socket is created with mode `0600`; only the service account and explicitly authorized local processes should be able to connect.
- The protocol uses ASCII text and LF (`\n`) line framing. Ordinary commands use one request per connection. `SUBSCRIBE` keeps a connection open after its acknowledgement; subscribed clients receive asynchronous `EVENT` lines. `PUBLISH` broadcasts event metadata to currently connected subscribers and then acknowledges the publisher.
- No network TCP listener is involved in this IPC channel.

## Request format

```text
v1 <request_id> <command>\n
```

Fields are separated by one or more ASCII whitespace characters when parsed by the current implementation. The request must contain exactly three fields before the line terminator.

- `v1`: protocol version. Unsupported versions are rejected.
- `request_id`: 1–31 ASCII characters from `[A-Za-z0-9_-]`. The server echoes valid IDs in responses.
- `command`: `PING`, `STATUS`, `EVENT`, `SUBSCRIBE`, or `PUBLISH`. `EVENT` and `PUBLISH` add two arguments: a UUID event ID and a lowercase event type (up to 100 characters). `SUBSCRIBE` has no arguments.
- Maximum request content before LF: 254 bytes (the LF itself is not counted). Larger/incomplete requests are rejected.

## Response format

```text
v1 <request_id> <status> <payload>\n
```

Status is `OK` or `ERROR`. Payload values for the current commands and errors are fixed tokens.

| Request command | Expected response |
|---|---|
| `PING` | `v1 <request_id> OK PONG\n` |
| `STATUS` | `v1 <request_id> OK RUNNING\n` |
| `EVENT <event_id> <event_type>` | `v1 <request_id> OK ACCEPTED\n` when metadata validates |
| `SUBSCRIBE` | `v1 <request_id> OK SUBSCRIBED\n`; connection remains open |
| `PUBLISH <event_id> <event_type>` | `v1 <request_id> OK PUBLISHED\n`; broadcasts to available subscribers |
| unknown command | `v1 <request_id> ERROR UNKNOWN_COMMAND\n` |
| invalid version/shape/ID or embedded NUL | `v1 0 ERROR BAD_REQUEST\n` |
| oversized request | `v1 0 ERROR REQUEST_TOO_LARGE\n` |

## Client rules

1. Connect to the configured Unix socket.
2. Write one complete ASCII request including LF.
3. Read until LF or EOF; enforce a short client-side deadline.
4. Validate the protocol version, echoed request ID, status, and payload.
5. For ordinary commands, close the connection and do not reuse it for a second request. A `SUBSCRIBE` connection remains open and receives `v1 push<N> EVENT <event_id> <event_type>\n` lines until disconnect.
6. Treat EOF before a complete response, malformed fields, or an unknown response token as a protocol error. Reconnect subscribers after disconnect and rely on PostgreSQL recovery scans for missed notifications. Do not retry non-idempotent future commands automatically unless their retry semantics are explicitly defined.

## Error and resource behavior

- The C server currently limits concurrent clients (including subscribers) to 64; new connections are closed when capacity is exhausted.
- Socket read/write are non-blocking and driven by `kqueue`.
- Current protocol errors are response tokens, not durable task failures.
- The server has no authentication beyond local socket filesystem permissions.
- There is no persistence or replay in the C core. `PUBLISH` is best-effort: disconnected subscribers miss notifications, and a subscriber with a pending write is skipped rather than blocking the event loop. PostgreSQL remains the durable source of truth; clients must retain recovery scans and make processing idempotent.

## Compatibility policy

- Do not change the meaning of existing v1 tokens incompatibly.
- Adding a new command requires updating this document, the shared C header, the C implementation, and tests in the same change.
- Any incompatible framing, field, or semantic change requires a new protocol version.
- The Go client should use the same token constants and contract rather than inferring undocumented behavior from implementation details.

## Current implementation note

The C implementation and Go IPC client are implemented and deployed on FreeBSD. The C event core uses `kqueue`/`kevent`; the Go API communicates with it over the local Unix socket. The C test suite passes PING, STATUS, EVENT validation, malformed requests, request-size limits, 24 concurrent clients, and SIGTERM shutdown. The production binary matches the source-built binary by SHA-256.


## Durable event intake (phase 1)

Business event intake is exposed by the Go API. `POST /events` validates an event and commits it to `portal.event_inbox` before returning `202 Accepted`; an identical replay of the same event ID/type/payload is reported as a duplicate, while reuse of an event ID with different content returns conflict. The IPC EVENT command validates event ID/type metadata only. It does not carry payload, claim an inbox row, persist the event in C, or execute business work. The Go `EventDispatcher` claims notification work with PostgreSQL leases, retries failed IPC notifications, and fences stale lease updates. The `EventProcessor` independently claims supported business events, stores results with the completed state, retries failures with backoff, and marks exhausted attempts failed. Periodic database scans recover missed notifications and expired processing leases; C push notifications remain best-effort wakeups. Neither HTTP receipt nor IPC ACCEPTED implies business completion.

Request JSON:

```json
{
  "event_id": "018f2f5e-7a2b-7c4d-8e9f-0123456789ab",
  "event_type": "content.published",
  "payload": {"source": "local", "record_id": "example"}
}
```

- `event_id` is a caller-generated UUID and is the idempotency key.
- `event_type` must start with a lowercase letter and contain only lowercase letters, digits, `.`, `_`, and `-` (maximum 100 characters).
- `payload` must be a JSON object. The HTTP request body is limited to 1 MiB.
- First receipt returns HTTP 202. An identical replay with the same ID/type/payload returns HTTP 200 and `duplicate: true`.
- Reusing an event ID with different type or payload returns HTTP 409. Invalid requests return HTTP 400; persistence failures return HTTP 503.
- Migration 003 adds independent dispatch state (`pending`, `leased`, `notified`), attempt count, lease expiry, retry schedule, last error, and notification timestamp. The Go dispatcher claims rows with `FOR UPDATE SKIP LOCKED`, sends the EVENT metadata envelope, retries failures with exponential backoff capped at 64 seconds, and reclaims expired leases after process interruption.
- `notified` means the C IPC endpoint returned `OK ACCEPTED` for event ID/type metadata only. It does not mean the payload reached C, was durably stored by C, or business work completed.
- Migration 004 adds independent processing lease fields. The Go `EventProcessor` claims only event types with explicitly registered handlers, using `FOR UPDATE SKIP LOCKED`; it marks `completed` only after the handler returns successfully, retries errors with exponential backoff, and marks an event `failed` after 10 attempts. Expired `processing` leases can be reclaimed.
- The registered production handler `portal.content.validate` validates a portal content payload and stores its result in `processing_result`. On 2026-10-15, a live end-to-end event completed successfully (`status=completed`, `dispatch_state=notified`, `attempts=1`, result `valid=true`). Replaying the same event ID/type/payload returned HTTP 200 with `duplicate: true`. Unknown event types are left untouched; this single successful test verifies the implemented path but is not a general load or failure-recovery certification.
- Processing is at-least-once: if a handler performs an external side effect and the process exits before `completed` is persisted, the handler may run again after lease expiry. Business handlers must be idempotent and honor their context deadline.
- The endpoint is loopback-bound along with the existing API. It is not an authenticated public ingress endpoint.


## Push notifications (phase 2, initial C implementation)

- `SUBSCRIBE` creates a persistent local stream and returns `SUBSCRIBED`.
- `PUBLISH <event_id> <event_type>` validates metadata and broadcasts `v1 push<N> EVENT <event_id> <event_type>` to connected subscribers.
- Broadcast is best-effort and never blocks the event loop on a slow subscriber. A subscriber with an outstanding write is skipped; reconnect/recovery must be handled by clients.
- The Go API maintains a reconnecting subscriber and turns push messages into buffered worker wakeups. The database dispatcher remains enabled at a 5-second interval as a durable recovery/dispatch fallback.
- The Node.js content API now sends `PUBLISH` after the content/event transaction commits (publish and published-version restore). Notification failure is logged but cannot roll back an already committed business transaction. Other event producers must use the same post-commit pattern when integrated.
