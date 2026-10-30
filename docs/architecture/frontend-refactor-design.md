# Frontend refactor design — 2026-10-29

> Historical design record for the Vue-era frontend dependency refactor. The first-stage Vue removal was completed on 2026-10-30 at `c31036badc52f58f251dde7f04958bb3ee55caef`; sections below describe the earlier implementation and experiments, not the current framework-free source tree. Use the current root README and second-stage Notion record for the active architecture baseline.

## Goal

Improve frontend module boundaries and make reusable dependencies independently cacheable through the existing `https://assets.lycupaynotion.top` static-resource service, without public CDNs, a new runtime service, or production changes.

## Non-goals

- No production deployment, Caddy/DNS change, service restart, or production HTML edit.
- No framework migration, router replacement, or API contract change.
- No widening of the initial JavaScript/CSS or lazy-feature size budgets to make a build pass.
- No forced splitting when the aggregate transfer size, runtime behavior, or cache invalidation becomes worse.

## Current evidence

- Baseline: `3521ab976c86a5cc2e6ee5459b6e1ca80b649399`; worktree branch: `refactor/frontend-asset-dependencies`.
- Baseline `npm run build` and `npm test` passed; 126 tests passed.
- Baseline initial entry JS: 66,056 gzip bytes, budget 66,560 bytes (about 504 bytes of headroom); initial CSS: 84,292 gzip bytes, budget 87,040 bytes.
- Baseline lazy editor JS: 685,868 raw / 211,791 gzip bytes; lazy Markdown chunk: 76,255 raw / 25,264 gzip bytes.
- `frontend/src/App.vue` is 1,288 lines / 101,098 bytes and still combines route lifecycle, authentication, resource operations, page state, and rendering. Existing composables already isolate several data loaders, editor actions, article actions, scroll restoration, and table/tag behavior.
- `build:static-origin` uses the owned asset origin as Vite base; deployment publishes hashed assets under the existing static-resource release. API paths remain root-relative and same-origin.

## Dependency boundary decision

1. Keep npm dependencies as build-time inputs; do not externalize bare package imports to the browser or introduce a public CDN.
2. Keep the existing editor and Markdown feature chunks independently lazy; their hashed assets are already published by the owned static-resource service. Do not externalize bare npm imports to browser import maps or a public CDN.
3. Reject the first vendor-chunk experiments: separating TipTap/ProseMirror caused the editor vendor to be eagerly preloaded and raised the combined gzip payload above its 230 KiB budget; separating `marked`/DOMPurify raised the Markdown feature aggregate from about 24.7 KiB to 36.3 KiB, above its 30 KiB budget. No thresholds were widened to accommodate these regressions.
4. Budget checks aggregate all modulepreloaded JavaScript and each feature's vendor chunks if a future experiment introduces them, rather than inspecting only a primary chunk. The current feature chunks remain outside the initial HTML preload graph.
5. Keep the Vue runtime in the current initial graph until a measured aggregate transfer-size comparison demonstrates that a separate runtime chunk respects the strict initial-load budget. A smaller entry file alone is not proof of a smaller initial transfer.
6. Bootstrap CSS and app CSS remain as currently built in this batch. Separating them requires a separate measured change that preserves CSS order, static-origin URL routing, and immutable-cache versioning.

## Module map and next source boundary

- `frontend/src/main.ts`: imports the ordered CSS entry points, initializes responsive layout, and mounts Vue.
- `frontend/src/App.vue`: owns navigation/session sequencing, page-level state, resource operations, and template rendering; split by one tested responsibility at a time. The first route parsing responsibility is extracted into `navigation/resolvePortalLocation.ts`; session/login/register/logout orchestration is extracted into `composables/usePortalAuthentication.ts`.
- `frontend/src/composables/load*.ts`: owns lazy data-loading flows and stale-response/error handling for the corresponding view.
- `frontend/src/composables/use*.ts`: owns focused reusable UI behavior; callers supply state/side-effect boundaries explicitly. `usePortalAuthentication.ts` owns session request sequencing and login/register/logout state transitions; it does not replace server-side authorization. Regression tests preserve the chosen login/register mode across navigation and ensure failed logout does not display a success notification.
- `frontend/src/components/*.vue`: view and interaction components; API authorization remains enforced by Go, not by hidden UI controls.
- `frontend/src/utils/markdown.ts`: sanitizes and renders Markdown; its dependency chunk remains lazy.
- `frontend/vite.config.ts`: build-time chunking and aliases only; no deployment side effects.
- `frontend/scripts/check-bundle-budget.mjs`: measured artifact budgets, including initial preload graph and lazy feature vendor chunks.

## Validation plan

For each batch: `npm run type-check`, `npm test`, `npm run build`, `npm run build:static-origin`, `npm run check:static-assets` or the matching static-origin layout check, and `git diff --check`. Inspect generated HTML and chunk dependency graph; verify no public CDN references, no editor/Markdown preloads, and same-origin `/api` requests. Chrome Test is optional and will be used only if its connected environment is available. No production deployment or service restart is part of this branch.

## Batch 3 — teaching-resource upload validation boundary (2026-10-29)

### Scope and behavior invariants

- Extract the teaching-resource form checks and pasted-file filtering from `App.vue` into `frontend/src/utils/teachingResourceValidation.ts`.
- Preserve existing user-facing error messages, supported extensions, the 25 MiB file-size limit, URL protocol restrictions, batch-upload behavior, and edit-without-replacement-file behavior.
- Load the validation utility only when the resource dialog submits or handles pasted files. Keep the initial JavaScript under the existing 65 KiB gzip budget; do not relax the budget.
- No API path, request payload, authorization behavior, upload transport, production file, or service is changed.

### Data/control flow

- Form submit: `App.vue#createTeachingResource` → lazy import of `validateResourceForm` → client-side error or existing XHR upload flow.
- Paste: `App.vue#handleResourcePaste` reads clipboard files and calls `preventDefault()` synchronously, then lazy-imports `filterPastedResourceFiles` → accepted files and rejection messages return to the existing form state.
- The utility is a user-feedback layer only. The Go API remains responsible for server-side file, URL, and permission validation.

### Boundary coverage

- `frontend/test/teaching-resource-validation.test.mjs` covers missing title/source, unsafe URL schemes, batch mode with no files, unsupported extension, the exact 25 MiB boundary, edits without replacement files, and mixed accepted/rejected clipboard files.
- An attempted extraction of the entire dialog state into a synchronous composable was rejected after measured builds exceeded the initial JS gzip budget (66,905 bytes against 66,560 bytes). That experiment was not retained. The smaller on-demand validation module reduced the initial payload instead of widening the threshold.

### Verification evidence

- `npm run type-check`: passed.
- `npm test`: 145/145 passed.
- `npm run build`: passed; initial JavaScript including modulepreloads 64.63 KiB / 65 KiB, initial CSS 82.32 KiB / 85 KiB, lazy editor 206.80 KiB / 230 KiB, lazy Markdown 24.67 KiB / 30 KiB.
- `npm run build:static-origin`: passed; initial JavaScript 64.65 KiB / 65 KiB, initial CSS 82.39 KiB / 85 KiB; static-origin layout check reported 2 entry URLs, 15 JS bundles, 5 CSS bundles and 74 icon URLs, with API paths remaining root-relative.
- `git diff --check`: passed. No public CDN references found in the checked frontend source/HTML.
- Production deployment, service restarts, GitHub Actions and Chrome Test were not run.

### Remaining risk / next boundary

- Browser clipboard integration and actual server-side upload validation were not exercised by these unit tests. Keep those limitations explicit.
- `App.vue` still owns resource dialog state, XHR upload transport, resource CRUD, and list refresh. Further extraction should move the UI and upload workflow together into a lazy-loaded dialog component or another boundary that measurably reduces initial payload; do not repeat a state-only extraction that breaches the current budget.

## Batch 4 — lazy teaching-resource editor dialog (2026-10-29)

### Goal and boundary

- Move the complete teaching-resource create/edit UI and upload workflow out of `App.vue` into `frontend/src/components/TeachingResourceEditorDialog.vue`.
- Keep `App.vue` responsible for opening the dialog, supplying the optional resource and option lists, synchronizing the shared resource busy flag, refreshing the list after a successful/partial save, and showing the completion notification.
- The child owns form fields, file selection/paste filtering, on-demand validation, same-origin XHR upload progress, partial batch outcomes, and completion UI. Server-side authorization and resource validation remain in the Go API.
- The component is loaded through `defineAsyncComponent` and mounted only while the dialog is open; it does not add an eager dependency to the initial route.

### Compatibility and control flow

- Create/edit endpoint and payload contract are unchanged: `POST /api/resources`, or `PATCH /api/resources/{id}`, with the existing `ResourceWrite` JSON shape and optional base64 file payload.
- Batch uploads still create independent resources per file; successful records trigger list refresh even when other files fail.
- Busy state is released before emitting the saved event, avoiding a race where list refresh and upload could overwrite the same loading flag.
- The close/escape/backdrop restrictions remain active while busy and after completion until the user clicks the explicit completion button.
- Public resource asset hosting, API paths, authentication behavior, and deployment topology are unchanged.

### Verification evidence

- `npm run type-check`: passed.
- `npm test`: 148/148 passed, including three component-boundary checks for lazy mounting, ownership of upload interactions, same-origin endpoints and on-demand validation.
- `npm run build`: passed; initial JavaScript including modulepreloads 61.75 KiB / 65 KiB gzip, initial CSS 82.32 KiB / 85 KiB gzip, lazy editor 206.80 KiB / 230 KiB gzip, lazy Markdown 24.67 KiB / 30 KiB gzip.
- `npm run build:static-origin`: passed; initial JavaScript 61.78 KiB / 65 KiB gzip, initial CSS 82.39 KiB / 85 KiB gzip; 16 JS bundles, 5 CSS bundles, 74 icon URLs; API paths remain root-relative.
- Browser clipboard interaction and live upload API behavior were not exercised by this batch's unit/architecture tests. No production deploy, service restart, merge, or Chrome Test was performed.

### Next candidate

- Review the remaining route/lifecycle orchestration in `App.vue` for a boundary with measurable benefit. Do not extract page state alone; only proceed when the ownership contract is clear and tests can protect navigation/session ordering and the existing initial bundle budget.


## Batch 5 — shared protected-route session guard (2026-10-29)

### Goal and boundary

- Extract the protected-route session gate into `frontend/src/composables/useProtectedRouteGuard.ts`.
- Reuse one contract from ordinary in-app navigation, browser `popstate`, and initial protected-route mounting. `App.vue` remains the owner of History API mutations, the route navigation sequence, route data loading, and the redirect presentation.
- The guard accepts injected route classification, session loading, a navigation freshness predicate, and a denial callback. It does not access the DOM, browser history, API transport, or global state. Go API authorization remains authoritative.

### Behavior invariants and failure paths

- Public route navigation proceeds without invoking the protected-route session gate; initial public pages still perform a background session load so the header can reflect an existing login.
- Protected routes do not load route data until session validation succeeds.
- An invalid session redirects the current navigation to `/login` and preserves the existing notice.
- A navigation already stale before the guard starts does not issue a session request. If a newer navigation starts while `/auth/me` is pending, the old completion cannot redirect the newer route.
- Existing `pushState`/`replaceState`, URL resolution, scroll restoration, route data loaders, and server API contracts are unchanged.

### Changed files and tests

- `frontend/src/composables/useProtectedRouteGuard.ts`: focused, framework-independent route-access policy.
- `frontend/src/App.vue`: ordinary navigation, `popstate`, and initial protected route now share the guard; duplicate redirect/session branches removed.
- `frontend/test/protected-route-guard.test.mjs`: public route, valid session, invalid session, stale-before-start, and stale-after-await cases.
- `frontend/test/navigation.test.mjs`: source-level integration assertions updated to require guard use before route data loading.

### Verification evidence

- Baseline before edits: `npm test` passed 148/148.
- After edits: `npm run type-check` passed; `npm test` passed 153/153.
- `npm run build` passed; initial JavaScript including modulepreloads 61.83 KiB / 65 KiB gzip; initial CSS 82.32 KiB / 85 KiB; lazy editor 206.80 KiB / 230 KiB; lazy Markdown 24.67 KiB / 30 KiB.
- `npm run build:static-origin` passed; initial JavaScript 61.85 KiB / 65 KiB gzip; initial CSS 82.39 KiB / 85 KiB; 16 JS bundles, 5 CSS bundles, 74 icon URLs, API paths remain root-relative.
- `npm run check:static-assets` passed (61 files, 1 HTML); `npm run test:static-assets` passed; `git diff --check` passed; checked frontend source/HTML public-CDN references: none.
- App.vue reduced from 1,143 lines before this batch to 1,128 lines. This is a secondary metric; the main result is one tested session-gate contract shared by three lifecycle paths.

### Limitations and rollback

- These tests exercise the guard contract with injected session/navigation callbacks; they are not a real-browser end-to-end test of browser history or a live `/auth/me` request.
- If regression is observed, revert this batch commit only; do not alter the production directory or widen bundle budgets.
- No production deployment, service restart, merge, GitHub Actions, or Chrome Test was performed.

### Next candidate

- Continue with the next high-risk business component from the Notion plan only after inspecting its callers, API contract, state ownership, error/retry paths, and existing tests. Do not continue extracting `App.vue` state solely to reduce line count.


## Batch 6 — compliance save lifecycle and busy-state recovery (2026-10-29)

### Goal and evidence-based defect

- Reviewed `frontend/src/components/ComplianceCenter.vue` before editing. Six write/verification flows manually set `saving = true`, awaited a mutation, performed follow-up reads, then set `saving = false` without a `finally` boundary.
- Concrete failure path: if a successful mutation is followed by a rejected `loadAssets`, `loadOverview`, `loadLicenses`, or `loadDocuments` request, control exits before the trailing assignment. The shared save buttons can remain disabled. This was a code-path finding, not a claim that a live production incident had occurred.
- Read-only backend review found integration coverage for non-admin denial, administrator access to the overview/list endpoints, CSV export response, and transactional rollback when compliance audit writing fails. The shared `api/openapi.yaml` does not define the compliance endpoints, so it was not treated as a complete contract source. No backend files or services were changed.

### Changes and invariants

- Added `frontend/src/utils/runWithBusyState.ts`, a framework-independent helper that sets busy before an async operation and always resets it in `finally`, while allowing errors to propagate to the caller.
- Routed asset updates, review creation, evidence creation/verification, license updates, and policy document updates through the shared busy-state helper.
- Added `refreshAfterSave` to distinguish a successful write from a failed follow-up refresh: the UI keeps the success result and reports that refresh failed and can be retried manually, instead of implying that the write itself failed.
- Restored the successful operation notice after detail reloads, because the shared `run()` helper clears prior notices when starting a read.
- Existing API paths, HTTP methods, request payloads, server-side permission checks, audit transaction behavior, and the policy that review actions do not automatically delete/unpublish content are unchanged.

### Tests and verification

- Added `frontend/test/run-with-busy-state.test.mjs`: busy state remains active during the operation, clears after success, clears after a rejected async operation, and clears after a synchronous throw.
- Extended `frontend/test/compliance.test.mjs` to verify all six mutation paths use the shared cleanup boundary and that refresh failure has an explicit user-facing message.
- `npm run type-check`: passed.
- `npm test`: 158/158 passed.
- `npm run build`: passed; initial JS (including modulepreloads) 61.83 KiB gzip / 65 KiB; initial CSS 82.32 KiB / 85 KiB; lazy editor 206.80 KiB / 230 KiB; lazy Markdown 24.67 KiB / 30 KiB.
- `npm run build:static-origin`: passed; initial JS 61.85 KiB gzip / 65 KiB; initial CSS 82.39 KiB / 85 KiB; 16 JS bundles, 5 CSS bundles, 74 icon URLs; API paths remain root-relative.
- `npm run check:static-assets`: passed (61 files, 1 HTML); `npm run test:static-assets`: passed.
- Frontend source/HTML public-CDN check: passed with no public CDN references; `git diff --check`: passed before commit.

### Limits and rollback

- Busy-state behavior is tested at the helper boundary and checked against the six component mutation functions; no real browser session or live compliance mutation was performed.
- If a regression is found, revert this batch commit only. Production directory and service state remain untouched.
- No deployment, service restart, merge, or GitHub Actions run.

### Next candidate

- Continue the Notion sequence with the system execution policy and system operations centers, first mapping frontend role visibility against backend RBAC and testing denial/error paths. Do not rely on UI gating as an authorization control; the Go API remains authoritative.

> Historical implementation record: the system execution-policy and privileged command-task feature described below was fully retired on 2026-10-31 by project-owner decision. The code, routes, UI, worker, service and database objects are removed in the current source; this section remains only to preserve the original engineering history.

## Batch 7 — preserve execution-policy mutation errors during reconciliation (2026-10-29; historical, feature retired 2026-10-31)

### Goal and evidence-based defect

- Reviewed `SystemExecutionPolicyCenter.vue`, `SystemOpsCenter.vue`, `PermissionManagement.vue`, account navigation references, and the Go API's system-ops/RBAC authorization entry points before changing code.
- Found a concrete error-reporting defect in `saveMode`: after a failed `PUT /system-ops/policy`, the catch block called `loadPolicy()`. That loader clears the shared error at entry and clears it again on a successful read, so a failed policy write could be presented with no error at all if reconciliation succeeded. The reload is useful to reconcile the selected mode with server state, but it must not erase the mutation failure.
- No role/permission mutation, API contract, route, server authorization, or database code was changed in this batch.

### Changes and invariants

- `SystemExecutionPolicyCenter.vue#saveMode` now captures the original save error, reloads policy state, then restores the save error. If reconciliation also fails, the message retains both the mutation failure and the reload failure.
- The `saving` flag remains released in `finally`; the write remains `PUT /system-ops/policy`, and the reconciliation read remains `GET /system-ops/policy`.
- Read-only RBAC review confirmed `/admin/*` handlers call `requireSuperAdmin` before dispatch. The backend requires both the `super_admin` role and the designated administrator account; user-role and role-permission mutations retain backend validation and audit writes.
- Read-only system-ops review confirmed every `/system-ops/*` request passes `requireSystemOpsAdmin`; GET/HEAD requests require `portal.system_ops.read`, while mutations require `portal.system_ops.manage`. Frontend visibility is not an authorization boundary.
- `SystemOpsCenter.vue` continues to use same-origin API calls and a credentialed same-origin EventSource. No client-side role check was added because the server remains authoritative and a reliable frontend capability contract would require a separate design/test seam.

### Tests and verification

- Added `frontend/test/system-execution-policy.test.mjs` to assert that save failure survives the reconciliation read, cleanup remains in `finally`, the mutation stays PUT, and the reconciliation request stays read-only GET.
- `npm run type-check`: passed.
- `npm test`: 160/160 passed.
- `npm run build`: passed; initial JavaScript including modulepreloads 61.82 KiB gzip / 65 KiB; initial CSS 82.32 KiB / 85 KiB; lazy editor 206.80 KiB / 230 KiB; lazy Markdown 24.67 KiB / 30 KiB.
- `npm run build:static-origin`: passed; initial JavaScript 61.85 KiB gzip / 65 KiB; initial CSS 82.39 KiB / 85 KiB; 16 JS bundles, 5 CSS bundles, 74 icon URLs; API paths remain root-relative.
- `npm run check:static-assets`: passed (61 files, 1 HTML); `npm run test:static-assets`: passed; `git diff --check`: passed; frontend source/HTML public-CDN check: no matches.

### Limits and rollback

- The new frontend tests are source-level boundary checks, not a mounted Vue test or live API test. The Go authorization paths were inspected read-only; backend integration tests were not run in the production checkout.
- No browser E2E, live session, or mutation was performed. No production deployment, service restart, merge, or GitHub Actions run.
- Rollback is limited to this batch's component and test changes; the production directory and services remain untouched.

### Next candidate

- Inspect `PermissionManagement.vue` post-save reconciliation feedback and account navigation's role visibility contract. Keep server-side RBAC authoritative; do not introduce client-side authorization assumptions without a verified session/capability source and tests.

## Batch 8 — stale log response protection in SystemOpsCenter (2026-10-29)

### Goal and defect

- Reviewed the log lifecycle in `frontend/src/components/SystemOpsCenter.vue`. Switching between server and SSH log sources starts a new request, but the older request can resolve later and replace the newer source's list/cursor or set the connection state based on the old request. Its `finally` could also clear the loading indicator while the new request is still pending.
- This was a source-level asynchronous race finding; no live incident or production behavior was inferred.

### Changes and invariants

- Added a monotonically increasing `logLoadSequence` and captured the requested source at the start of `loadLogs`.
- Successful and failed responses now check both the sequence and active source before mutating logs, cursor, errors, or connection state.
- The loading flag is cleared only by the latest request. Unmount increments the sequence to invalidate any pending completion and closes the stream.
- Endpoint paths, request parameters, source selection UI, cursor semantics, EventSource credential behavior, and backend authorization remain unchanged.

### Verification

- Added source-boundary assertions for the sequence token and invalidation contract to `frontend/test/system-execution-policy.test.mjs`.
- `npm run type-check`: passed; `npm test`: 161/161 passed.
- `npm run build`: passed; initial JS including modulepreloads 61.82 KiB gzip / 65 KiB; initial CSS 82.32 KiB / 85 KiB; lazy editor 206.80 KiB / 230 KiB; Markdown 24.67 KiB / 30 KiB.
- `npm run build:static-origin`: passed; initial JS 61.85 KiB gzip / 65 KiB; initial CSS 82.39 KiB / 85 KiB; 16 JS bundles, 5 CSS bundles, 74 icon URLs; API paths remain root-relative.
- Static asset layout and checker tests passed; `git diff --check` passed. No public CDN references were found in frontend source/HTML.

### Limits and rollback

- Tests assert the source-level lifecycle contract; no mounted Vue test, simulated deferred HTTP response, live EventSource, or browser E2E was performed.
- No production deployment, service restart, merge, or GitHub Actions run. Production files and services remain untouched.

### Next candidate

- Add a testable request-sequencing helper or a mounted component test with deferred promises if the test environment can support it without adding an eager dependency. Then inspect `PermissionManagement.vue` reconciliation feedback and account navigation role visibility.


## Batch 9 — permission editor selection locking (2026-10-29)

- Reviewed `PermissionManagement.vue`, account navigation visibility, and Go RBAC handlers. During a pending PATCH, account buttons and the role selector remained active while checkboxes and save buttons were disabled. The live role label used after `await` could therefore refer to a different role than the one updated. This is a source-level race finding, not a live incident.
- Disabled account selection buttons and the role selector while `saving` is true. API paths, PATCH methods, request payloads, super-admin safeguards, and server authorization are unchanged.
- Read-only Go review confirmed `/admin/*` requires `requireSuperAdmin`; role mutations validate role/permission keys and write audit records transactionally. No frontend visibility policy was changed because that would be an access-policy decision.
- Added `frontend/test/permission-management.test.mjs` for selection lock and API/UI contract assertions.
- Verification: type-check passed; 163/163 tests passed; regular and static-origin builds passed. Initial JS gzip 61.82/65 KiB regular, 61.85/65 KiB static-origin; CSS 82.32/85 KiB and 82.39/85 KiB. Static asset checks and `git diff --check` passed; no public CDN references found.
- Limits: source-level tests only; no mounted Vue, live API, or Go integration test. Production untouched; no deploy, restart, merge, or GitHub Actions.
- Next: review `NotificationHost.vue` and `utils/notification.ts` timer/queue lifecycle.

## Batch 10 — notification timer lifecycle cleanup (2026-10-29)

### Goal and evidence

- Reviewed `frontend/src/components/NotificationHost.vue`, `frontend/src/utils/notification.ts`, all notification call sites, and `frontend/test/notification.test.mjs` before editing.
- The notification manager owns a global reactive queue and a `Map` of timeout handles. Both manual dismissal and queue overflow route through `dismiss`, so eviction is expected to clear the removed item's timer as well as remove its UI state.
- The cleanup check used truthiness (`if (timer)`) even though timer handles are opaque values. A falsy handle would be treated as absent and skipped by `clearTimeout`. The change now checks presence explicitly (`timer !== undefined`). This is a defensive lifecycle correction, not a claim that a production timer failure was observed.

### Changes and invariants

- Added module/function documentation describing the utility's state ownership, caller relationship, queue cap, and timer behavior.
- Updated timer cleanup to distinguish a missing map entry from a falsy handle.
- Added a deterministic regression test that stubs timer scheduling and asserts that manual dismissal and oldest-item eviction clear their associated timers, including a falsy handle.
- Preserved queue cap of five, oldest-first eviction, default 4.2-second duration, `duration <= 0` persistent-notification behavior, notification titles/messages, accessible live-region markup, and all public call signatures.
- No changes to `NotificationHost.vue` were necessary after reviewing its component lifecycle: it subscribes to the shared state and delegates explicit close actions; timer ownership remains in the notification manager rather than the rendering component.

### Verification evidence

- `npm run type-check`: passed.
- `npm test`: 164/164 passed.
- `npm run build`: passed; initial JS including modulepreloads 61.83 KiB gzip / 65 KiB; initial CSS 82.32 KiB / 85 KiB; lazy editor 206.80 KiB / 230 KiB; lazy Markdown 24.67 KiB / 30 KiB.
- `npm run build:static-origin`: passed; initial JS 61.85 KiB gzip / 65 KiB; initial CSS 82.39 KiB / 85 KiB; 16 JS bundles, 5 CSS bundles, 74 icon URLs; API paths remain root-relative.
- `npm run check:static-assets`: passed (61 files, 1 HTML); `npm run test:static-assets`: passed.
- `git diff --check`: passed; frontend source/HTML scan found no public CDN references.

### Limits and rollback

- Timer cleanup is tested with a deterministic scheduler stub plus the existing real-time auto-dismiss test; this is not a mounted Vue/browser lifecycle test.
- No production deployment, service restart, merge, or GitHub Actions run. Production files and services remain untouched.

### Next candidate

- Continue with `TeachingResourceDrawer.vue`, `VirtualizedSelect.vue`, and `utils/responsiveLayout.ts` in the planned order; first map component ownership, input validation, accessibility, listener/style-variable cleanup, and large-data behavior before selecting one independently testable boundary.


## Batch 11 — responsive layout listener/request lifecycle (2026-10-29)

### Goal and evidence

- Reviewed `TeachingResourceDrawer.vue`, `VirtualizedSelect.vue`, `utils/responsiveLayout.ts`, `main.ts`, CSS token definitions, and existing test conventions.
- The responsive initializer registered global `resize` and `orientationchange` listeners but exposed no cleanup function. Its debounce timer also used truthiness to decide whether to clear a timer, and the timer reference was not reset after firing. An in-flight fetch could still update document root styles after the owner had been disposed because no disposal boundary existed.
- Selected this as the smallest high-value lifecycle seam; drawer formatting already has a shared utility, while changing virtualized-select keyboard/focus semantics needs dedicated mounted-component tests and was deferred.

### Changes and invariants

- `initializeResponsiveLayout()` now returns a cleanup callback that removes both global listeners, clears any defined debounce timer, resets timer ownership, and increments the request sequence so in-flight fetch/JSON responses become stale.
- Debounce cleanup now uses explicit `resizeTimer !== undefined`, then clears the reference when the scheduled callback starts.
- Added source-boundary tests for cleanup exposure, listener removal, in-flight request invalidation, debounce timer ownership, and existing CSS write restrictions.
- Preserved the 160ms debounce, same-origin GET, version=1 policy requirement, `--layout-` CSS variable allowlist, and CSS fallback on errors. `main.ts` still initializes once before Vue mount; no runtime call-site behavior changes.

### Verification evidence

- `npm run type-check`: passed.
- `npm test`: 167/167 passed.
- `npm run build`: passed; initial JS including modulepreloads 61.87 KiB gzip / 65 KiB, initial CSS 82.32 KiB gzip / 85 KiB, lazy editor 206.80 KiB gzip / 230 KiB, lazy Markdown 24.67 KiB gzip / 30 KiB.
- `npm run build:static-origin`: passed; initial JS 61.89 KiB gzip / 65 KiB, initial CSS 82.39 KiB gzip / 85 KiB; 16 JS bundles, 5 CSS bundles, 74 icon URLs; API paths remain root-relative.
- `npm run check:static-assets`: passed (61 files, 1 HTML); `npm run test:static-assets`: passed.
- `git diff --check`: passed; checked frontend source/HTML has no public CDN references.

### Limits and constraints

- Current tests are source-boundary assertions, not browser-level event/fetch lifecycle tests. The returned cleanup API enables a future mounted/jsdom behavior test.
- No production changes, service restarts, deployment, PR merge, or manual GitHub Actions.

## Batch 12 — preserve labels for valid empty-string select values (2026-10-29)

### Goal and evidence

- Reviewed `VirtualizedSelect.vue`, its two call sites in `App.vue` and `TeachingResourceEditorDialog.vue`, the option mappings, and the existing Node test setup before editing.
- The trigger previously rendered `selected?.label || placeholder`. `Option.label` is typed as `string`, so an intentionally empty label was treated as if no option were selected and replaced with the placeholder. The component can distinguish a missing selected option (`undefined`) from a selected option whose label is `''`; nullish fallback preserves that distinction. The existing grade option's empty-string *value* is not itself the defect because its label is non-empty; it was reviewed to verify that model values and display labels are separate concerns.
- This is a source-contract edge case identified by tracing the shared component and its callers, not a claim of a confirmed production incident.

### Changes and invariants

- Changed the trigger label fallback from `selected?.label || placeholder` to `selected?.label ?? placeholder`, so fallback applies only when no option label exists; an explicitly empty label is preserved.
- Added component-level documentation for responsibilities, state ownership, collaborators, and the valid empty-string value boundary.
- Added `frontend/test/virtualized-select.test.mjs` to guard nullish display fallback and the caller-owned `modelValue` / `update:modelValue` contract.
- Kept option filtering, virtualization window calculations, scroll reset, outside-click/Escape listeners, selected value emission, styling, and all public props unchanged. No new dependencies were installed.

### Verification evidence

- `npm run type-check`: passed.
- `npm test`: 169/169 passed.
- `npm run build`: passed; initial JavaScript including modulepreloads 61.87 KiB gzip / 65.00 KiB budget; initial CSS 82.32 KiB gzip / 85.00 KiB budget; lazy editor 206.80 KiB gzip / 230.00 KiB budget; lazy Markdown 24.67 KiB gzip / 30.00 KiB budget.
- `npm run build:static-origin`: passed; initial JavaScript including modulepreloads 61.89 KiB gzip / 65.00 KiB budget; initial CSS 82.39 KiB gzip / 85.00 KiB budget; lazy editor 206.80 KiB gzip; lazy Markdown 24.67 KiB gzip; 16 JS bundles, 5 CSS bundles, 74 icon URLs; API paths remain root-relative.
- `npm run check:static-assets`: passed (61 files, 1 HTML); `npm run test:static-assets`: passed; `git diff --check`: passed; frontend source/HTML public-CDN scan found no matches.

### Limits and constraints

- The new tests are source-boundary assertions. They do not mount the Vue component or test actual browser rendering, keyboard navigation, focus behavior, or pointer events. The component's broader keyboard/focus semantics remain a separate candidate requiring mounted-component coverage.
- No production changes, service restarts, deployment, PR merge, or GitHub Actions run. Bundle budgets were not changed.

### Next candidate

- Before changing keyboard navigation or focus semantics in `VirtualizedSelect.vue`, establish mounted-component coverage using the already-installed jsdom and Vue runtime/compiler tooling; then cover open/close, disabled-state transitions, empty results, option updates, and keyboard focus behavior without changing caller-owned selection semantics.

## Batch 13 — VirtualizedSelect mounted behavior and keyboard/filter lifecycle (2026-10-29)

### Goal and evidence

- Continued the prior batch's deferred work by replacing source-only checks for the shared `VirtualizedSelect.vue` with mounted Vue component tests using the repository's existing `jsdom`, `@vue/compiler-sfc`, `esbuild`, Vite, and Vue packages. No dependency was installed.
- The mounted tests reproduced a keyboard event path issue: the search input uses `@keydown.stop`, which prevents the document-level Escape listener from receiving Escape while focus is in the input. The previous test dispatched Escape directly on `document`, so it did not cover the actual focused control.
- Review of the virtualization calculations also identified a stale-scroll edge case: narrowing the filtered options while the viewport is scrolled could preserve a `scrollTop` beyond the new result count, leaving the visible window empty even when matching options exist.

### Changes and invariants

- Added `onSearchKeys` to close the popover on Escape from the search field while retaining event propagation isolation for other keys.
- Added a watcher on the filtered option list to reset the component scroll index and DOM viewport scroll position when search results/options change.
- Reworked `frontend/test/virtualized-select.test.mjs` into a jsdom-mounted Vue harness. It now exercises opening/filtering, selection emission and model updates, query reset after selection/reopen, empty results, Escape from the actual search input, narrowing a scrolled list, outside-click close, disabled state, option updates, and unmount listener cleanup. Retained source assertions for nullish label fallback and caller-owned model contract.
- Preserved public props/events, option identity, selection semantics, row/viewport dimensions, same-origin application architecture, and bundle budgets.

### Verification evidence

- `npm run type-check`: passed.
- `npm test`: 174/174 passed, including six tests dedicated to `VirtualizedSelect`.
- `npm run build`: passed; initial JS including modulepreloads 61.90 KiB gzip / 65.00 KiB budget; initial CSS 82.32 KiB gzip / 85.00 KiB budget; lazy editor 206.80 KiB gzip / 230.00 KiB budget; lazy Markdown 24.67 KiB gzip / 30.00 KiB budget.
- `npm run build:static-origin`: passed; initial JS 61.92 KiB gzip / 65.00 KiB budget; initial CSS 82.39 KiB gzip / 85.00 KiB budget; 16 JS bundles, 5 CSS bundles, 74 icon URLs; API paths remain root-relative.
- `npm run check:static-assets`: passed (61 files, 1 HTML); `npm run test:static-assets`: passed; `git diff --check`: passed; frontend source/HTML public-CDN scan found no matches.

### Limits and constraints

- These are jsdom component DOM/event tests, not visual cross-browser tests or screen-reader/assistive-technology tests. No live API or production browser session was used.
- No production changes, deployment, service restart, PR merge, or manual GitHub Actions run. Bundle budgets were not changed.

### Next candidate

- Review `TeachingResourceDrawer.vue` data formatting and lifecycle against its callers and existing shared utilities; only change behavior when a reproducible contract or lifecycle issue is found, then add a focused test before implementation.


## Batch 14 — TeachingResourceDrawer 数据格式化边界（2026-10-29）

### 发现与修改

- 按上一批候选审查 `TeachingResourceDrawer.vue`、`App.vue` 调用点、生成的 `TeachingResource` 类型与 `utils/resourceFiles.ts`。父组件持有资源数据/视图模式并处理 close/edit/download；详情抽屉只负责展示和转发用户意图，没有需要新增的持久状态。
- 抽屉原先复制了一份文件大小格式化逻辑，并以 `fileSize` 真值判断是否展示。合法的 `0` 字节会被当作缺失值显示为 `—`；复制的格式化函数还会把 0 字节格式化成 `1 KB`。现改用共享的 `formatResourceFileSize`，并让有效的 0 字节显示为 `0 B`，缺失/非法大小仍显示 `—`。
- 创建/更新时间直接调用 `new Date(value).toLocaleString()`；格式错误的时间戳会渲染为误导性的 `Invalid Date`。新增局部 `formatResourceDate`：缺失值显示 `—`，有效日期采用 `zh-CN` 24 小时格式，无效日期保留原始值以便发现数据问题，而不伪装成有效日期。
- 组件模板按职责拆分为可读结构，补充职责、状态所有权、协作工具和数据边界注释。没有改变 API、父组件状态所有权、资源链接 `noopener noreferrer`、活动/回收站操作可见性或事件签名。

### 行为测试与验证

- 新增 `frontend/test/teaching-resource-drawer.test.mjs`，使用仓库现有 Vue SFC compiler、Vue runtime、esbuild 与 jsdom 挂载真实组件；没有安装新依赖。
- 测试覆盖 0 字节文件、无效创建/删除时间戳、回收站不显示编辑/下载操作、close/edit/download 事件与资源 ID、外链 `target`/`rel` 安全属性及外链动作标签。
- `npm run type-check`：通过。
- `npm test`：178/178 通过。
- `npm run build`：通过；首屏 JS（含 modulepreload）61.91 KiB gzip / 65.00 KiB 预算，首屏 CSS 82.32 KiB / 85.00 KiB，懒加载编辑器 206.80 KiB / 230.00 KiB，Markdown 24.67 KiB / 30.00 KiB。
- `npm run build:static-origin`：通过；首屏 JS 61.93 KiB gzip / 65.00 KiB，首屏 CSS 82.39 KiB / 85.00 KiB；16 个 JS bundle、5 个 CSS bundle、74 个 icon URL；API 路径仍为根相对路径。
- `npm run check:static-assets`：通过（61 个文件、1 个 HTML）；`npm run test:static-assets`、`git diff --check` 均通过；前端源码/HTML 公共 CDN 扫描无匹配。

### 限制与约束

- jsdom 验证组件 DOM/事件与边界格式化，不替代真实浏览器的视觉、跨浏览器和辅助技术测试；没有调用生产 API。
- 生产目录未修改；未部署、未重启服务、未合并 PR、未运行 GitHub Actions；bundle 预算未放宽。

### 下一批候选

- 继续审查资源列表/详情数据加载和视图切换的竞态及父子状态同步；先从 `loadTeachingResourceList.ts` 和调用方建立可复现的请求序列测试，只在确认边界问题后修改。


## Batch 15 — 教学资源列表请求竞态回归测试（2026-10-29）

### 审查结论与变更

- 对照工程标准检查 loadTeachingResourceList.ts、App.vue 中的动态加载器/视图切换调用方、同类 loadContentList.ts，以及已有的 loader 测试。
- 当前实现已通过单调递增 request sequence 和捕获的 view mode 阻止过期结果写入；不需要仅为制造代码差异而改写实现。本批重点补齐此前未直接覆盖的并发搜索失败和 active/trash 交错请求场景。
- 扩展 frontend/test/teaching-resource-list-loader.test.mjs 两项确定性测试：较早的搜索请求在较新的搜索成功后失败时，不得覆盖新数据、写入错误或触发通知；active 请求与 trash 请求交错时，过期 active 响应不得污染回收站数据，最终 loading 状态应正确释放。
- 不改 API 路径、请求语义、状态所有权、通知策略或生产代码。通过验证当前请求序列保护，而不是无证据地引入取消请求/AbortController。

### 验证证据

- 专项 loader 测试：5/5 通过。
- npm run type-check：通过。
- npm test：180/180 通过。
- npm run build：通过；首屏 JS（含 modulepreload）61.91 KiB gzip / 65.00 KiB 预算；首屏 CSS 82.32 KiB gzip / 85.00 KiB；懒加载编辑器 206.80 KiB gzip / 230.00 KiB；Markdown 24.67 KiB gzip / 30.00 KiB。
- npm run build:static-origin：通过；首屏 JS 61.93 KiB gzip / 65.00 KiB；首屏 CSS 82.39 KiB gzip / 85.00 KiB；16 个 JS bundle、5 个 CSS bundle、74 个 icon URL；API 路径仍为根相对路径。
- npm run check:static-assets：通过（61 个文件、1 个 HTML）；npm run test:static-assets 通过；前端源码/HTML 公共 CDN 扫描无匹配；git diff --check 通过。

### 限制与安全边界

- 本批新增的是可控 Promise 顺序测试，不是浏览器网络/真实后端集成测试；不声称 API 请求被取消，只验证 stale response 不会提交到 UI 状态。
- 生产目录未修改；未部署、未重启服务、未合并 PR、未运行 GitHub Actions；bundle 预算未放宽。

### 下一批候选

- 检查 App.vue 对动态导入 loader 初始化失败与请求失败的错误/忙碌状态处理是否重复或可能产生重复通知；用可测的纯函数或最小集成测试先确认调用路径，再决定是否抽取公共错误边界。

## Batch 16 — 动态 loader 初始化失败后的可重试性（2026-10-29）

### 审查结论与证据

- 按 Batch 15 的候选，检查 `App.vue` 中内容列表与教学资源列表 loader 的动态导入、两类 loader 内部的请求错误处理、错误通知 watcher/回调及现有测试。
- 两处调用方均将动态导入 Promise 缓存到模块级变量。若 chunk 下载或模块初始化失败，该 rejected Promise 会永久留在缓存中；之后用户再次进入列表时会复用同一个 rejected Promise，无法重新尝试加载模块。请求执行失败与初始化失败的边界不同：列表 loader 已初始化后会自行处理 API 请求错误并释放 loading，外层 catch 主要负责初始化阶段失败，不应将 API 错误处理重复抽象。
- 因此没有创建泛化的公共错误边界；只抽取一个可独立测试的 `createRetryableLazyLoader`，用于缓存成功初始化、共享并发初始化，并在初始化 Promise reject 时清除缓存，允许下一次调用重试。

### 变更与不变量

- 新增 `frontend/src/utils/retryableLazyLoader.ts`，无 Vue/browser 依赖；成功结果持续缓存，失败初始化不会被永久缓存，原始异常继续传递给调用方。
- `App.vue` 的内容列表与教学资源列表动态 loader 改用该 helper。请求 loader 内部的 API 错误状态、教学资源通知回调、内容错误 watcher、loading 状态所有权和调用签名保持不变。
- 新增 `frontend/test/retryable-lazy-loader.test.mjs`，验证并发调用共享同一 Promise、成功初始化只执行一次、reject 后下一次重试成功，以及 factory 同步抛错也能重试。
- 没有改变 API 路径、认证/权限边界、路由、请求取消策略或 bundle 预算；没有新增依赖。

### 验证证据

- `npm run type-check`：通过。
- `npm test`：183/183 通过。
- `npm run build`：通过；首屏 JS（含 modulepreload）61.93 KiB gzip / 65 KiB 预算，首屏 CSS 82.32 KiB gzip / 85 KiB，懒加载编辑器 206.80 KiB gzip / 230 KiB，Markdown 24.67 KiB gzip / 30 KiB。
- `npm run build:static-origin`：通过；首屏 JS 61.95 KiB gzip / 65 KiB，首屏 CSS 82.39 KiB gzip / 85 KiB；16 个 JS bundle、5 个 CSS bundle、74 个 icon URL，API 路径仍为根相对路径。
- `npm run check:static-assets`：通过（61 个文件、1 个 HTML）；`npm run test:static-assets`、`git diff --check` 均通过；前端源码/HTML 公共 CDN 扫描无匹配。

### 限制与安全边界

- 单元测试验证注入式 Promise factory 的缓存/重试语义；未模拟浏览器真实 chunk 网络故障或线上动态导入行为，也未调用生产 API。
- 生产目录未修改；未部署、未重启服务、未合并 PR、未运行 GitHub Actions；bundle 预算未放宽。

### 下一批候选

- 审查其他 `App.vue` 中缓存的动态模块初始化 Promise（例如 public directory/article loader 和 content-management actions），核对它们是否存在相同的永久 rejected-promise 缓存；按具体副作用和调用契约逐个测试，不做未经证实的批量替换。

## Batch 17 — 其余动态数据模块初始化失败恢复（2026-10-29）

### 审查结论与变更

- 延续 Batch 16 的证据检查，对 `App.vue` 全部 Promise 缓存/动态导入点做仓库级检索。剩余的 `useContentManagementActions`、`loadPublicDirectoryData` 和 `loadPublicArticle` 仍使用 `promise ??= import(...).then(...)`；这些模块初始化失败后同样会永久缓存 rejected Promise。
- 将这三处初始化器也改为 `createRetryableLazyLoader`，与内容列表和教学资源列表保持同一缓存契约。已初始化后的业务请求错误仍由对应 composable 自己处理，不会触发 loader 模块重新初始化。
- 公开目录加载器有额外边界：原先 `loadPublicContents` / `loadPublicTeachingResources` 没有捕获初始化阶段的异常；从路由/搜索 watcher 以 `void` 调用时可能产生未处理 rejection，而且错误状态无法解释失败原因。本批为这两个调用点增加初始化失败 catch，分别写入已有错误 ref 并确保 loading 复位。正常 API 请求错误仍由 `loadPublicDirectoryData.ts` 内部处理，因此不会重复通知或覆盖业务错误。
- 更新 `frontend/test/vite-code-splitting.test.mjs` 的架构契约测试，覆盖五个懒加载数据模块都使用可重试初始化，并检查公开目录初始化错误会设置错误状态、释放 loading。未改动 `defineAsyncComponent` 的组件加载器，因为它由 Vue 管理且重试/错误处理语义不同，不应未经单独验证混用此 helper。

### 变更范围与不变量

- 修改 `frontend/src/App.vue`、`frontend/test/vite-code-splitting.test.mjs` 和本架构记录。
- 动态导入仍然保持按需分包；API URL、认证/授权、请求序列保护、通知机制、页面路由和数据所有权不变。
- 没有新增依赖，没有改变 API 请求重试策略；这里只允许重新尝试模块初始化，不自动重放任何业务 API 请求。

### 验证证据

- `npm run type-check`：通过。
- `npm test`：184/184 通过（Batch 17 动态模块初始化边界契约测试）。
- `npm run build`：通过；首屏 JS（含 modulepreload）61.93 KiB gzip / 65 KiB 预算，首屏 CSS 82.32 KiB gzip / 85 KiB，懒加载编辑器 206.80 KiB gzip / 230 KiB，Markdown 24.67 KiB gzip / 30 KiB。
- `npm run build:static-origin`：通过；首屏 JS 61.95 KiB gzip / 65 KiB，首屏 CSS 82.39 KiB gzip / 85 KiB；16 个 JS bundle、5 个 CSS bundle、74 个 icon URL，API 路径仍为根相对路径。
- `npm run check:static-assets`：通过（61 个文件、1 个 HTML）；`npm run test:static-assets`、`git diff --check` 均通过；前端源码/HTML 公共 CDN 扫描无匹配。

### 限制与安全边界

- 架构契约测试检查 `App.vue` 的源码接线；通用 retry helper 的 Promise 缓存/失败恢复行为由 Batch 16 的确定性单元测试覆盖。本批未模拟真实浏览器 chunk 网络故障，也未进行生产 API/浏览器集成测试。
- 生产目录未修改；未部署、未重启服务、未合并 PR、未运行 GitHub Actions；bundle 预算未放宽。

### 下一批候选

- 检查 `App.vue` 路由切换与公开内容加载的生命周期是否会在模块初始化失败后产生重复提示或状态串扰；以现有 route/navigation sequence 为基线，先补充可复现测试，再决定是否调整调用协调方式。

## Batch 18 — 公开文章过期请求不得污染新路由状态（2026-10-29）

### 复现与根因

- 在 `loadPublicArticle.ts` 中，成功响应在提交文章/HTML 前同时校验 request sequence 和捕获的 route；但 `catch` 与 `finally` 只校验 request sequence。
- 通过可控 Promise 测试复现：文章请求未完成时导航离开文章页，随后请求 reject，旧请求仍会把错误写进共享 `publicContentError`（可能触发不相关的错误通知），并将共享 `publicContentLoading` 清为 false。该 loading ref 也被公开目录加载复用，旧文章请求可能错误地结束新路由的 loading 状态。
- 测试先于实现添加，首轮运行失败，实际错误文本为 `stale article request failed`，确认不是推测性修复。

### 修改

- 将文章 loader 的 catch/finally 写状态条件统一为 `requestId === requestSequence && options.route.value === requestedRoute`。
- 过期路由上的旧请求不再写入错误或释放由新路由拥有的 loading 状态；当前路由的真实请求失败仍会报告错误并正常释放 loading。
- 新增 `frontend/test/public-article-loader.test.mjs` 回归用例，模拟文章请求在离开文章页后失败，同时让共享 loading ref 归属新路由，验证错误不串扰、loading 不被旧请求清除。

### 验证证据

- 新回归测试在修复前失败，修复后通过；`npm run type-check` 通过。
- `npm test`：185/185 通过。
- `npm run build`：通过；首屏 JS（含 modulepreload）61.93 KiB gzip / 65 KiB 预算，首屏 CSS 82.32 KiB gzip / 85 KiB，懒加载编辑器 206.80 KiB gzip / 230 KiB，Markdown 24.67 KiB gzip / 30 KiB。
- `npm run build:static-origin`：通过；首屏 JS 61.95 KiB gzip / 65 KiB，首屏 CSS 82.39 KiB gzip / 85 KiB；16 个 JS bundle、5 个 CSS bundle、74 个 icon URL，API 路径仍为根相对路径。
- `npm run check:static-assets`：通过（61 个文件、1 个 HTML）；`npm run test:static-assets`、`git diff --check` 均通过；前端源码/HTML 公共 CDN 扫描无匹配。

### 限制与安全边界

- 可控 Promise 测试覆盖异步时序和共享 ref 所有权，不等于真实浏览器网络或生产 API 集成测试；没有引入请求取消或改变 API 行为。
- 生产目录未修改；未部署、未重启服务、未合并 PR、未运行 GitHub Actions；bundle 预算未放宽。

### 下一批候选

- 对照同样的 route + request sequence 不变量检查法律文档和其他共享 loading/error ref 的 loader，区分已被测试覆盖的保护与真实缺口，不做无证据的统一重构。

## Batch 19 — 公开目录请求离开当前视图后的状态隔离（2026-10-29）

### 复现与根因

- 延续 Batch 18 对共享 `publicContentLoading` / `publicContentError` 的审查，检查 `loadPublicDirectoryData.ts` 与文章 loader 的协作。目录 loader 只有自己的搜索 request sequence，不知道当前路由是否仍由目录页拥有。
- 使用可控 Promise 先添加测试，复现公开目录请求未完成时切换到文章路由、随后旧请求 reject 的情况：旧请求仍写入 `stale directory request failed`，并可能清除新文章加载的共享 loading 状态。另补充成功响应场景，验证旧目录结果也不能提交到已切换的视图。

### 修改

- 为 `createPublicDirectoryLoader` 增加可选的 `isContentViewCurrent` 判定，默认返回 true，保持独立调用方和既有测试的兼容性。
- 公开内容请求在提交列表/页大小、写错误或释放共享 loading 前，除 request sequence 外，还需满足当前仍处于公开内容目录视图。
- `App.vue` 注入当前视图判定：主页、知识目录和其他公开内容目录页共享数据；教育资源页与文章详情页不被误判为内容目录。
- 新增两项回归测试：过期目录失败不污染错误/loading；过期目录成功不覆盖当前视图数据，也不清除新路由的 loading 状态。
- 公开教育资源使用独立的 loading/error refs，本批不改变其请求语义；没有引入取消请求或重放 API 的逻辑。

### 验证证据

- 两个新增回归测试均在实现前复现失败，接入 route guard 后通过。
- `npm run type-check`：通过；`npm test`：187/187 通过。
- `npm run build`：通过；首屏 JS（含 modulepreload）61.95 KiB gzip / 65 KiB 预算，首屏 CSS 82.32 KiB gzip / 85 KiB，懒加载编辑器 206.79 KiB gzip / 230 KiB，Markdown 24.67 KiB gzip / 30 KiB。
- `npm run build:static-origin`：通过；首屏 JS 61.97 KiB gzip / 65 KiB，首屏 CSS 82.39 KiB gzip / 85 KiB，懒加载编辑器 206.80 KiB gzip / 230 KiB；16 个 JS bundle、5 个 CSS bundle、74 个 icon URL，API 路径仍为根相对路径。
- `npm run check:static-assets`：通过（61 个文件、1 个 HTML）；`npm run test:static-assets`、`git diff --check` 均通过；前端源码/HTML 公共 CDN 扫描无匹配。

### 限制与安全边界

- 以注入式路由判定和可控 Promise 验证视图所有权；不替代浏览器真实导航/网络集成测试。请求仍会完成，只是过期响应不再修改当前视图状态。
- 生产目录未修改；未部署、未重启服务、未合并 PR、未运行 GitHub Actions；bundle 预算未放宽。

### 下一批候选

- 审查法律文档、公开资源和其他共享 refs 的路由/请求所有权边界；只有发现可复现的跨视图写入才修改，并保留各 loader 的既有请求语义。
