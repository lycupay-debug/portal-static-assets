# Experiment log

For each experiment, record: baseline commit, hypothesis, files changed, exact command, exit status, output measurements, regressions, and conclusion. Separate measured results from assumptions. Never describe an unrun check as passed.

## 2026-10-28 — baseline build and initial layout check

- Source baseline: `7d36218343da4a74a29f788b219231e65f3264f2`.
- Environment: FreeBSD 15.1-RELEASE-p6, Node.js v24.21.0, npm 12.0.2.
- Dependency install: `npm ci --no-audit --no-fund` succeeded in this isolated clone. npm reported that the `esbuild` install script was blocked by the current `allowScripts` policy; the subsequent build nevertheless completed successfully.
- Type check: `npm run type-check` passed.
- Unit tests: `npm test` passed, 126 tests, 0 failures.
- Production-mode frontend build: `npm run build` passed, including existing bundle budgets.
- Static layout check: passed, 59 output files and one HTML entry file.
- Baseline measurements from this run: initial JavaScript 189.50 KiB raw / 64.51 KiB gzip; initial CSS 575.70 kB raw for the emitted `index` CSS (bundle-budget accounting reported 82.32 KiB gzip); lazy editor JavaScript 669.79 KiB raw / 206.83 KiB gzip; lazy Markdown renderer 24.67 KiB gzip.
- Important: Vite's console emitted `vite v8.3.4`, while `package.json` declares `vite ^8.2.2`; lockfile resolution is the reproducible dependency source. Do not compare this run with another environment unless Node/npm versions and lockfile are aligned.
- Scope: all commands ran under `/opt/portal-static-assets`; no production deployment, service restart, or Caddy change was performed.

## 2026-10-28 — static package and CSS reference validation

- Expanded the read-only layout checker to validate local HTML `src`/`href` references and CSS `url(...)` references, reject symlinks, and detect paths escaping the output directory.
- Added isolated regression tests for valid references, missing HTML assets, missing CSS assets, and path traversal references.
- Added a release packager that creates a commit-named tarball containing `site/` plus a per-file SHA-256 manifest, and emits an archive SHA-256 sidecar. It refuses overwrite and performs no upload.
- Added manual CI steps to test the checker, validate build output, package the release candidate, and upload the package as a short-lived workflow artifact. Automatic CI remains disabled as in the selected source baseline.
- The current deployment destination and URL/base-path strategy remain undecided; no production deployment was attempted.
