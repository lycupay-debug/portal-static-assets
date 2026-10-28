# Frontend refactor design — 2026-10-29

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
