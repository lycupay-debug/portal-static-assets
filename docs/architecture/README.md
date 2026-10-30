# Architecture documents

This directory records the current system architecture, static-resource publishing design, scope boundaries, build output conventions, and verification criteria. Decisions must be grounded in the current source tree and build behavior; development deployment must not silently become a production release.

- `system-map.md`: current frontend, Go API, PostgreSQL, C event-core, Caddy, file-storage, trust boundaries, data flows, test coverage and remaining phase-2 work.
- `static-resource-publishing.md`: initial architecture and scope.
- `release-package.md`: integrity-protected release candidate format.
- `static-origin-deployment.md`: selected independent static-origin URL/base strategy and a production deployment plan that has not been applied.
