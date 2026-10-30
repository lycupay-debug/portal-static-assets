# Backend API

## Single Go business backend

The only production business HTTP API is the Go service in api/main.go and api/portal_api.go. Caddy strips the public /api prefix and proxies to 127.0.0.1:8081. The retired Node.js implementation is excluded from this repository and preserved outside it at `/opt/personal-portal-legacy-node` solely for emergency rollback. It must remain stopped and disabled after a successful Go-only cutover.

Go owns:
- Registration, password hashing, login/logout, session cookie lifecycle and authentication.
- Content CRUD, publish/unpublish, public reads, likes/favorites, version snapshots, restore and recycle bin.
- Teaching resource metadata, URL-only resources, file uploads/downloads, recycle bin and permanent deletion.
- Event intake, dispatch leases, retries, business handlers, processing results and recovery.
- Standard JSON error responses and PostgreSQL transactions.

C only validates and forwards bounded event metadata over the local Unix Socket. It never receives business payloads. PostgreSQL remains the durable source of truth.

## HTTP routes

The complete contract is api/openapi.yaml. The public API includes:
- /auth/register, /auth/login, /auth/me, /auth/logout
- /uploads
- /content, /content/{id}, /content/{id}/publish, /content/{id}/unpublish, /content/{id}/versions, /content/{id}/versions/{versionId}/restore, /content/trash, restore and permanent-delete routes
- /public/content, /public/content/{slug}, /public/content/{slug}/interactions
- /resources, /resources/{id}, /resources/{id}/download, restore and permanent-delete routes
- /layout-policy and /healthz

The internal Go endpoints /events, /dbz, /readyz and /metrics are intentionally denied by public Caddy routing. They are used by local integrations, deployment checks and monitoring.

## API contract

api/openapi.yaml is the source contract. TypeScript types are generated with @hey-api/openapi-ts into frontend/src/api/generated. Change the OpenAPI schema first, regenerate types, and require CI to confirm generated output is current. Redocly lint validates the OpenAPI document.

Business API errors use JSON with an error field and an optional errors array. Content parent/child taxonomy is independently stored and validated in Go and PostgreSQL; explicit null values clear both taxonomy fields, while mismatched pairs are rejected.

## Event lifecycle and IPC v2

- POST /events commits event ID/type/payload to portal.event_inbox before returning 202. Identical replays return 200 with duplicate=true; conflicting reuse returns 409.
- EventDispatcher claims notification work with PostgreSQL leases and publishes ID/type metadata to C over IPC v2. Failed publication is retried with exponential backoff; lease fencing prevents stale dispatchers from overwriting newer results.
- C supports PING, STATUS, EVENT, SUBSCRIBE, PUBLISH and ACK. PUBLISHED confirms a fan-out attempt. ACKNOWLEDGED confirms Go received a matching push. Neither is business completion.
- Each subscriber has one outstanding push and a bounded FIFO of 32 metadata notifications. Queue overflow, disconnects and core restarts may lose in-memory hints. Go's periodic PostgreSQL scans and expired-lease recovery remain enabled.
- EventProcessor claims registered event types independently, runs handlers, persists processing_result and commits terminal status. Production handlers include portal.content.validate and portal.file.cleanup. Handlers must be idempotent because processing is at-least-once.
- Permanent deletion of an article image or uploaded teaching resource inserts a portal.file.cleanup event in the same PostgreSQL transaction as the business-row deletion. A rollback therefore cannot leave an orphaned cleanup task, and a committed deletion cannot lose its cleanup task if the API process stops.
- File cleanup payloads contain only a managed-file kind and a generated UUID filename, never an arbitrary path. The handler resolves only the configured upload/resource directory, treats a missing file as success, and returns filesystem errors to the existing 10-attempt exponential-backoff retry path. Expired leases recover after process restarts; exhausted tasks remain status=failed for inspection.
- The internal metrics endpoint exposes personal_portal_file_cleanup_events_failed for monitoring. The event processor emits event_dead_lettered when a handler exhausts its retry attempts. Operators should alert on a non-zero failed gauge and inspect portal.event_inbox.last_error before manually requeueing a failed event.

## Local verification

From the repository root:

    cd frontend
    npm run api:lint
    npm run api:generate
    npm run type-check
    npm test
    npm run build

    cd ../backend
    go test -race ./...
    go vet ./...
    go build ./api

    cd ../event-core
    make clean all test

For PostgreSQL integration tests, set PERSONAL_PORTAL_TEST_DATABASE_URL_FILE to a dedicated test database URL file after applying all migrations. Never point the full integration suite at the live database while the production worker is running.

## Deployment

Use deploy/freebsd/deploy.sh. It requires a clean Git checkout, runs tests/builds, creates a full database backup, applies pending migrations, installs Go/C binaries, switches the frontend and Caddy routes, runs health checks, then stops and disables the Node.js compatibility service. The emergency rollback copy is stored outside the repository at `/opt/personal-portal-legacy-node`. deploy/freebsd/rollback.sh restores the previous binaries, Caddyfile and frontend release using the deployment timestamp.


System operations:
- `GET /system-ops/warnings`, `/system-ops/logs`, and `/system-ops/stream` are restricted to accounts with `portal.system_ops.read`; hiding the navigation entry is not treated as authorization.
- The privileged command-task executor, root worker, developer/stable mode policy, task queue, approval endpoints, and UI have been retired. Migration 031 drops the associated database tables and obsolete `portal.system_ops.manage` permission. Public `/events` intake continues to reject the retired `portal.command.execute` event name so stale clients cannot enqueue unsupported work. System administration through this portal is read-only for warnings/logs plus account RBAC management; host maintenance commands are not exposed as a portal feature.
- Log history is read from a bounded tail, then incrementally from byte cursors. The SSE stream checks for appended lines every two seconds only while a browser is subscribed, sends keepalives, and supports `Last-Event-ID` resume. The UI closes the stream when the page is unmounted or paused.
- Server messages come from `/var/log/messages`. GPT/SSH operations use a separate sanitized projection at `/var/log/host-mcp-admin-audit.log`; raw shell commands and output remain in the root-only `/var/log/host-mcp-audit.log` and are not exposed through the portal API.
- Only the sanitized SSH audit projection is readable by the `personalportal` service account. Log lines are bounded in the browser to 500 entries; the API initial tail is capped at 256 KiB and 200 entries per response.
