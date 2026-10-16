# Event Core IPC Contract v2

## Status

IPC v2 adds explicit acknowledgement for subscriber push delivery and a bounded per-subscriber FIFO. v1 clients are rejected. This protocol carries event metadata only; business payloads and durable state remain in Go and PostgreSQL.

## Transport and framing

- Transport: Unix Domain Socket, AF_UNIX / SOCK_STREAM.
- Default socket path: /var/run/personal-portal/event.sock.
- Socket mode is 0600. Only the service account and explicitly authorized local processes may connect.
- Frames are ASCII, LF-delimited. Request lines are limited to 254 bytes before LF.
- Ordinary commands use one request per connection. SUBSCRIBE stays open for pushes and ACK exchanges.
- Request format: v2 <request_id> <command> [event_id event_type]
- Response format: v2 <request_id> <status> <payload>
- Request IDs are 1–31 ASCII characters from [A-Za-z0-9_-]. Event IDs are UUIDs. Event types are lowercase identifiers up to 100 characters.

## Commands and acknowledgement semantics

| Command | Response | Meaning |
|---|---|---|
| PING | OK PONG | Process/socket connectivity only |
| STATUS | OK RUNNING | C event loop is accepting IPC |
| EVENT id type | OK ACCEPTED | Metadata validates; no durable receipt or processing promise |
| SUBSCRIBE | OK SUBSCRIBED | Opens a persistent notification stream |
| PUBLISH id type | OK PUBLISHED | C accepted a metadata fan-out attempt; not proof all subscribers received it |
| ACK id type | OK ACKNOWLEDGED | The matching pending notification was read by Go; not proof business processing completed |

All responses echo the request ID. Invalid requests return ERROR BAD_REQUEST; unknown commands return ERROR UNKNOWN_COMMAND; oversized requests return ERROR REQUEST_TOO_LARGE; stale, mismatched, or duplicate subscriber acknowledgements return ERROR BAD_ACK. The old v1 version returns BAD_REQUEST.

## Push delivery and duplicate notifications

- Push frame: v2 push<N> EVENT <event_id> <event_type>.
- Each subscriber may have one outstanding unacknowledged push. Go sends ACK with the exact event ID and event type.
- Further notifications enter a per-subscriber FIFO with capacity 32. This queue contains metadata only, is in memory, and is discarded on core restart or subscriber disconnect.
- If the bounded queue overflows, the core logs the event and may drop that notification. Go still has a durable event row and runs a periodic recovery scan; therefore notifications are wake-up hints, not the queue of record.
- Duplicate notifications can wake the same worker more than once. PostgreSQL leases, idempotency keys, status checks, and transactional completion prevent a duplicate wake from being treated as a second durable business event.
- A lost ACK causes the subscriber to reconnect. Any unacknowledged in-memory push may be redelivered or lost with the connection; Go must reconcile work from PostgreSQL rather than rely on replay from C.
- On C restart, subscribers disconnect and Go reconnects with exponential backoff capped at 10 seconds. The in-memory FIFO is lost. Expired leases and periodic database scans recover unfinished work.

## Timeout and resource policy

- Go command timeout is 750 ms by default; caller contexts may impose a shorter deadline.
- The Go subscriber uses bounded line reads, validates version/shape/event metadata, acknowledges each push, and reconnects after any EOF, timeout, malformed frame, or ACK error.
- C uses non-blocking sockets and kqueue/kevent; maximum concurrent clients is 64. A subscriber cannot block the event loop on a slow write.
- Each subscriber FIFO is capped at 32 entries. Overflow is observable through logs and recovered by the durable database dispatcher/processor.
- The C core has no authentication beyond local socket permissions and does not expose a TCP listener.

## Business responsibility and persistence

- Go persists payloads to portal.event_inbox before publication.
- Go owns authentication, validation, idempotency, processing leases, retries, business handlers, processing results, and final status.
- C never receives, parses, stores, or executes payload JSON. It only validates bounded metadata fields and relays notification frames.
- PUBLISHED and ACKNOWLEDGED are delivery-level protocol receipts. Only a committed PostgreSQL processing result and terminal status prove business completion.
- The Go EventDispatcher uses PostgreSQL leases and sends PUBLISH; the Go subscriber turns pushes into buffered worker wakeups. The EventProcessor independently claims supported events using database leases. A wakeup may be duplicated or lost without losing durable work.

## Compatibility and tests

Any incompatible framing or semantic change requires a new protocol version and a matching update to the C header, C implementation, Go client, documentation, and tests.

Automated C tests cover v2 acceptance, v1 rejection, malformed/oversized requests, metadata validation, FIFO ordering across outstanding ACKs, successful and duplicate/mismatched ACKs, concurrent clients, and SIGTERM shutdown. Go tests cover command response validation, subscriber ACKs, reconnects, and real C-core restart recovery on FreeBSD.
