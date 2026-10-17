# Data processing and retention fact review — 2026-10-18

## Purpose and evidence boundary

This document records facts visible in the current repository's Go API and SQL migrations. It is a technical fact sheet for preparing the privacy notice, not the published privacy policy or a legal opinion. Hosting, backups, CDN account settings, production logs, database access policy, and off-host data flows cannot be established from application source alone.

## Confirmed in source

| Data class | Source-level fields / handling | Known purpose | Retention status |
|---|---|---|---|
| Account | Email, display name, user UUID, bcrypt password hash, created/updated timestamps | Registration, authentication, account management | No account deletion/closure policy established by this review. |
| Login session | Session UUID, user UUID, SHA-256 token hash, created/expiry timestamps; `portal_session` cookie is Secure, HttpOnly, SameSite=Strict, path `/api`, 30-day Max-Age | Maintain login state | Expired session rows are deleted when a new session is created. No independent scheduled purge was found in the reviewed repository. |
| User content | Author ID, title, slug, content type, tags/categories, summary/body, visibility/status, publication and audit timestamps | Save, edit, publish, manage and display content | Soft deletion and version history exist in the application; complete retention/deletion lifecycle requires confirmation. |
| Article interactions | User ID, content ID, interaction type (like/favorite), created timestamp | Provide like/favorite features and aggregate counts | No independent retention schedule established by this review. |
| Education resources | Owner ID, title/description/type/grade, public flag, external URL, original/stored filename, MIME type, file size, timestamps and soft-delete timestamp | Store, manage, preview and distribute user-submitted resources | Resource files are stored separately from the main content database. Soft delete and permanent deletion endpoints exist; backup/log retention remains unknown. |
| Inline images | Authenticated image upload accepts PNG/JPEG/GIF/WebP/AVIF up to 8 MiB and stores a UUID-named file under the configured upload directory; response exposes an `/uploads/<name>` path | Embed images in content | The inspected endpoint does not associate the file with an owner/content record or create a rights-review record at upload time. A dedicated image lifecycle and rights gate is needed. |
| Copyright complaints | Name, email, role, work description, proof-of-rights text, suspected URLs, requested action, truthfulness declaration, status, handling notes, assignee, received/updated/resolved timestamps, retention deadline | Handle rights complaints, but only after the public intake is enabled | Intake is gated on published complaint/privacy documents and `PERSONAL_PORTAL_COMPLAINT_RETENTION_DAYS` (30–3650). A recurring purge/retention job was not found in the reviewed repository; do not enable intake until retention execution and policy are aligned. |
| Operational logs | Reviewed source contains event identifiers/types, reconnect durations and error details in event-core subscriber logs. | Reliability diagnosis and event processing | This does not establish all application, Caddy, OS, database, CDN or security-log fields or their retention. Verify actual configuration and log rotation on the host. |

## Controls already present

- Passwords are stored as bcrypt hashes, not plaintext.
- Session tokens are stored as SHA-256 hashes, and session cookies use Secure, HttpOnly and SameSite=Strict.
- The complaint intake route remains disabled until both the complaint policy and privacy policy are published and a valid retention-day setting exists.
- The code refuses to publish a policy when required facts remain missing or the approval information is incomplete.
- Resource files have soft-delete and permanent-delete paths; public resource access checks the public flag and deletion state.

## Gaps that block publishing a factual privacy policy

1. Confirm the legal identity and contact channel of the personal-information handler.
2. Verify the actual FreeBSD host, database, resource storage, backup and disaster-recovery locations and providers.
3. Inspect production Caddy, system, database and Cloudflare settings to establish logged fields, access restrictions, retention and any cross-border processing.
4. Define a real account closure/deletion workflow and retention schedule for content, interactions, uploads, backups and security logs.
5. Implement and test automatic deletion of expired complaint records and associated evidence after the approved retention deadline, including safe handling of legal holds.
6. Associate inline image uploads with the uploading user and content, track their hashes and rights evidence, and prevent public serving until the publication gate passes.
7. Verify whether any analytics, external scripts, embedded third-party content, email service or other processors are active in the deployed frontend/runtime.
8. Confirm the process for access/correction/deletion requests and, if applicable, the handling of minors' personal information.

## Required implementation sequence

- Keep all public legal documents in draft while factual blockers remain.
- Add an auditable retention job in report-only/dry-run mode first; do not delete complaint material until the retention period and legal-hold rules are approved.
- Implement image ownership/lifecycle and rights-gate integration with tests covering upload, draft, publish, unpublish, edit, delete, restore, and direct URL access.
- Verify actual host configuration and log rotation before writing concrete retention or location claims into the privacy policy.
- After the controls pass tests and production checks, update the compliance ledger with evidence, review owner, and review date.
