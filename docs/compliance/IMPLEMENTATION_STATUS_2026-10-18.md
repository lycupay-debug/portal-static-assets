# Copyright compliance implementation status — 2026-10-18

This file records the deployed technical state. It does not replace legal advice or approve any asset's rights.

## Deployed controls

- Production release: `20261017T171207Z-go-backend`; source commit `373dd1d`.
- PostgreSQL migration 019 (`uploaded_image_rights_gate`) is applied.
- Local uploads are registered with uploader, SHA-256, MIME type and size. New images default to unknown rights / pending review and are private.
- Caddy proxies `/uploads/*` to the Go API instead of serving the upload directory directly. The API permits private preview for the uploader or admin; anonymous delivery requires an approved rights record, verified evidence, and a reference from at least one currently published public article.
- The article publish/restore gate checks both local uploaded images and recognized external image URLs. External images require a matching compliance asset and verified rights evidence.
- The compliance center can preview registered private uploaded images while an authorized reviewer is signed in.
- Daily compliance audit now emits a report-only retention inventory and backup/upload filesystem counts. No deletion is performed.
- Public health and readiness endpoints passed after deployment; Caddy, API and event-core services are running.
- GitHub Actions for commit `373dd1d` passed: https://github.com/lycupay-debug/personal-portal/actions/runs/39052652465

## Latest observed production inventory

- 221 candidate assets in the latest scanner output; the merged database `portal.compliance_assets` ledger currently contains 237 rows, including records retained from prior scans and manual review.
- Risk triage: 9 high, 2 medium, 171 low, 39 unknown, 0 critical. These are review priorities, not final legal conclusions.
- 7 externally hosted image URLs remain recorded as high-priority review items.
- 5 files remain in the upload directory but have no rows in the new uploaded-image registry; direct public requests return 404. They were not deleted.
- 46 files exist in the backup directory at the latest report; oldest observed modification time is 2026-10-14T18:14:20Z. Automatic backup deletion is not configured by this project.
- The latest retention dry-run found 0 expired sessions, 0 soft-deleted content records, 0 soft-deleted resources, 0 complaints with an expired retention deadline, and 0 complaints missing a deadline. This is a snapshot, not a retention-policy approval.

## Items intentionally left unresolved

- The public article titled “阿松大” remains published. The scanner reports that it contains text and page elements corresponding to a Snaper Digital CRM article, plus 7 externally hosted images. Authorization has not been verified. This remains a high-priority copyright risk, not a final infringement determination. No content was automatically removed or unpublished.
- The four public legal documents remain drafts. Operator identity/contact details, actual provider data handling and retention, hosting/backup regions, applicable filing/licensing status and legal review remain to be confirmed.
- The license catalog still contains six license-expression candidates marked unreviewed. Automated metadata and captured license text are not legal approval.
- Complaint intake remains disabled until the complaint and privacy documents are approved and published, a retention period is configured, and retention/legal-hold execution is implemented.
- No automatic purge is enabled for complaints, recycle-bin content, old uploads, or backups. The database does not yet provide a legal-hold model.
- `docs/compliance/PUBLISH_GATE.md` contains an older pre-administrator statement. For the current operational workflow, use `docs/compliance/OPERATIONS.md` and the latest implementation status; do not rely on the stale administrator sentence as current state.

## Next engineering priorities

1. Keep existing high-risk public content visible in the compliance register until the owner makes the rights/disposition decision; do not infer authorization from test intent or remote availability.
2. Add legal-hold support before implementing destructive retention automation.
3. Finish provider/hosting/logging fact verification and obtain legal review before publishing public policies.
4. Conduct browser-based visual acceptance of the compliance center and verify authorized preview in a signed-in session.
