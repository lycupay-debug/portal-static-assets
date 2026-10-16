# Retired Node.js API implementation

This directory contains the historical Node.js API implementation retained temporarily for emergency rollback and behavioral comparison only.

The production target is the single Go API in backend/api/portal_api.go. The Caddy production route must send /api/* to 127.0.0.1:8081; the Node.js service on port 8082 must remain stopped and disabled in rc.conf. New business features, validation rules, database behavior, and API changes belong only in Go and api/openapi.yaml.

Do not add new behavior here. After the Go-only deployment has passed its observation window, this legacy source and its Node-specific tests can be removed in a separate cleanup commit. Git history and the deployment backups retain rollback evidence.
