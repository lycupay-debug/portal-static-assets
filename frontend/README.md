# Frontend architecture and development

This directory contains the portal's framework-free TypeScript/DOM frontend. Vue SFCs, the Vue runtime, the Vue Vite plugin, and Vue-specific type checking have been removed in the first refactor stage.

## Runtime architecture

- `src/main.ts` is the browser entry point and imports the ordered CSS layers.
- `src/portalApplication.ts` creates and owns the portal application lifecycle.
- `src/navigation/` resolves locations, routes, and navigation transitions.
- `src/state/` contains framework-independent reactive state and state contracts; `@preact/signals-core` is the small reactive primitive.
- `src/composables/` contains focused data-loading and action workflows. The directory name is historical terminology; modules do not depend on Vue.
- `src/views/` and `src/components/` render standard DOM nodes and own explicit mount/dispose lifecycles.
- `src/api/` contains the same-origin Go API client and generated OpenAPI types.
- `src/utils/` contains pure or narrowly scoped helpers, including Markdown and resource validation.
- Tiptap Core / ProseMirror remain for rich-text editing. The Vue adapter is not used.

The Go API remains the only business and authorization authority. Frontend route guards and hidden controls are usability features, not security boundaries.

## Development

Requirements: Node.js 24 and npm.

```sh
npm ci
npm test
npm run type-check
npm run api:lint
npm run api:check
npm run build
npm run check:static-assets
```

The build runs frontend behavior tests, TypeScript project checks, Vite production bundling, and bundle-budget checks. Static-origin packaging checks are separate:

```sh
npm run build:static-origin
npm run package:static-origin
```

## Test strategy and limits

- `test/` contains Node.js/jsdom behavior and lifecycle tests.
- `e2e/` contains Playwright Chromium browser workflows.
- Unit/jsdom tests do not prove live API integration or real-browser behavior. Report E2E and production checks separately.
- Tests must assert user-visible behavior, state ownership, error paths, and disposal effects; do not add source-only assertions as a substitute when a behavior test is practical.

## Refactor rules

Before changing a module, trace its callers, dependencies, state ownership, asynchronous work, DOM listeners, cleanup, API contracts, and tests. Preserve public behavior unless the change explicitly targets it. New or substantially refactored files must document responsibilities, dependencies, collaborators, side effects, and important boundaries. Follow the repository's AI engineering standard and the current second-stage Notion record.
