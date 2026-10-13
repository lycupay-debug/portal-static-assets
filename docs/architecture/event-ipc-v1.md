# Event Core IPC Contract v1

## Status

Version 1 is the first local connectivity contract between the C event core and future backend clients. It currently supports only health/connectivity commands; it is not yet a business event submission protocol.

## Transport

- Transport: Unix Domain Socket, `AF_UNIX` / `SOCK_STREAM`.
- Default socket path: `/var/run/personal-portal/event.sock`.
- Development/test runs may pass an alternate socket path as the C program's only argument.
- The socket is created with mode `0600`; only the service account and explicitly authorized local processes should be able to connect.
- The protocol uses ASCII text, LF (`\n`) as the line terminator, and one request per connection. The server sends one response and then closes the connection.
- No network TCP listener is involved in this IPC channel.

## Request format

```text
v1 <request_id> <command>\n
```

Fields are separated by one or more ASCII whitespace characters when parsed by the current implementation. The request must contain exactly three fields before the line terminator.

- `v1`: protocol version. Unsupported versions are rejected.
- `request_id`: 1–31 ASCII characters from `[A-Za-z0-9_-]`. The server echoes valid IDs in responses.
- `command`: command token, currently `PING` or `STATUS`.
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
| unknown command | `v1 <request_id> ERROR UNKNOWN_COMMAND\n` |
| invalid version/shape/ID or embedded NUL | `v1 0 ERROR BAD_REQUEST\n` |
| oversized request | `v1 0 ERROR REQUEST_TOO_LARGE\n` |

## Client rules

1. Connect to the configured Unix socket.
2. Write one complete ASCII request including LF.
3. Read until LF or EOF; enforce a short client-side deadline.
4. Validate the protocol version, echoed request ID, status, and payload.
5. Close the connection and do not reuse it for a second request.
6. Treat EOF before a complete response, malformed fields, or an unknown response token as a protocol error. Do not retry non-idempotent future commands automatically unless their retry semantics are explicitly defined.

## Error and resource behavior

- The C server currently limits concurrent clients to 64; new connections are closed when capacity is exhausted.
- Socket read/write are non-blocking and driven by `kqueue`.
- Current protocol errors are response tokens, not durable task failures.
- The server has no authentication beyond local socket filesystem permissions.
- There is no persistence, replay, delivery guarantee, idempotency key, task queue, or business-event schema in v1.

## Compatibility policy

- Do not change the meaning of existing v1 tokens incompatibly.
- Adding a new command requires updating this document, the shared C header, the C implementation, and tests in the same change.
- Any incompatible framing, field, or semantic change requires a new protocol version.
- The Go client should use the same token constants and contract rather than inferring undocumented behavior from implementation details.

## Current implementation note

The current C implementation accepts a 255-byte input buffer including a maximum of 255 bytes before the line terminator in the intended contract, but the implementation's read buffer is 256 bytes and reserves a NUL terminator. Keep tests authoritative and reconcile the exact boundary if this limit changes. The Go backend is not implemented yet; this document defines the contract it must follow.


## Durable event intake (phase 1)

Business event intake is currently exposed by the Go API, not by the local C IPC v1 protocol. `POST /events` validates an event and commits it to `portal.event_inbox` before returning `202 Accepted`. The C process is still only a local event/health core at this stage; dispatch/notification from PostgreSQL-backed intake into the C event loop is a subsequent phase and must not be implied by a successful HTTP receipt.

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
- `portal.event_inbox` stores receipt status and basic processing fields, but phase 1 does not yet claim dispatch, retries, worker leases, or completion semantics.
- The endpoint is loopback-bound along with the existing API. It is not an authenticated public ingress endpoint.
