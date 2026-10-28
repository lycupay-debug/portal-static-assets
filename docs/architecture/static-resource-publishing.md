# Static resource publishing — initial investigation

## Goal

Evaluate a repeatable and independently verifiable way to publish frontend static assets, while preserving the current portal's behavior and keeping production unchanged.

## Non-goals

- No production deployment or service restart.
- No Caddy/DNS/firewall changes.
- No immediate switch in production asset URLs or caching policy.
- No removal of existing frontend build-budget checks.

## Initial observations to verify

- Frontend build is driven by Vite; the baseline `frontend/vite.config.ts` has no explicit `manualChunks` strategy.
- `frontend/package.json` includes `build`, `build-only`, `type-check`, and test scripts; the `build` script also runs a bundle-budget check.
- Build output currently uses the baseline's existing Vite defaults unless overridden elsewhere.

## Investigation sequence

1. Map the frontend entry points, imports, dynamic imports, public assets, and build scripts.
2. Record baseline build output and bundle-size measurements without touching production output.
3. Test candidate chunking and asset-directory conventions in this isolated repository only.
4. Add deterministic checks for expected output layout, asset references, and size budgets.
5. Compare clean-build results and document trade-offs before proposing any production adoption.

## Acceptance criteria

- Baseline commit is recorded and reproducible.
- Experiments are reproducible from documented commands.
- Existing type-check, tests, and bundle-budget checks remain available.
- Output asset references resolve in the generated build.
- No production files, configuration, or services are modified.
