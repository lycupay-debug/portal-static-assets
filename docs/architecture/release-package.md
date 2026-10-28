# Static asset release package

## Current scope

This repository can build and package a static-site release candidate. Packaging is not deployment: it does not upload files, edit Caddy configuration, switch production asset URLs, or restart services.

## Build and verify

From `frontend/`:

```sh
npm ci
npm run test:static-assets
npm run build
npm run check:static-assets
npm run package:static-assets
```

The package script writes an archive under `artifacts/` named for the current Git commit, plus a SHA-256 sidecar. The archive contains `site/` (the Vite output) and `manifest.json`; the manifest records each file's relative path, size, SHA-256, the project commit, and the selected source baseline.

## Safety properties

- Refuses to overwrite an existing archive for the same commit.
- Rejects symlinks in the generated site.
- Records per-file integrity hashes and an archive checksum.
- Does not perform network uploads or production changes.
- The manual GitHub Actions workflow uploads the archive as a CI artifact for 14 days; it does not deploy it.

## Deployment decision still required

The target host/path and asset URL strategy must be chosen before actual publication. The current frontend uses root-relative paths such as `/icons/bootstrap-icons/*.svg`; a separate origin or path prefix needs an explicit compatibility strategy. Until the target is decided and validated, do not switch the production portal to the new assets.
