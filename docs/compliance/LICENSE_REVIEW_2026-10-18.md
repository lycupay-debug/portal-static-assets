# Frontend production dependency license review — 2026-10-18

## Scope and method

This is a source-level preliminary review of the committed `frontend/package-lock.json` and `frontend/public/THIRD_PARTY_NOTICES.txt` on `main`. The production closure was computed from lockfile entries not marked development-only. It is not a legal opinion and does not, by itself, change the production database's license review statuses.

Snapshot:
- 95 production dependency instances.
- 95 package/version sections in `THIRD_PARTY_NOTICES.txt`.
- 6 license expressions in the production closure.
- No dependency has been marked legally approved in the database by this document.

## Production closure

| SPDX expression | Package instances | Example package | Preliminary obligations to check |
|---|---:|---|---|
| MIT | 90 | Vue, Tiptap extensions, Bootstrap | Retain copyright notice, permission notice and disclaimer in copies/substantial portions. |
| Apache-2.0 | 1 | TypeScript 6.0.3 | Include license text; preserve applicable notices; review any upstream NOTICE file and modified-file notices; patent and trademark provisions are distinct from copyright permission. |
| BSD-2-Clause | 1 | entities 7.0.1 | Retain copyright notice, conditions and disclaimer in source; reproduce them in documentation/materials for binary distribution. |
| ISC | 1 | picocolors 1.1.1 | Retain copyright and permission notice in copies/substantial portions. |
| BSD-3-Clause | 1 | source-map-js 1.2.2 | BSD-2-style notice retention plus the no-endorsement clause. |
| (MPL-2.0 OR Apache-2.0) | 1 | DOMPurify 3.4.17 | The package offers alternatives. The installed package's `LICENSE` file and the notice section contain Apache-2.0 text; record this as the current distribution choice only after the project owner/reviewer confirms the choice. If MPL is chosen instead, perform the applicable file-level copyleft review. |

Counts are package instances, not unique upstream projects. Package metadata and a matching license file are evidence of the declared license, not proof that all distribution conditions have been met.

## High-value review notes

1. The current production closure contains no GPL/LGPL/AGPL expression in the lockfile-derived production set. This statement applies only to this frontend production closure, not the complete host, n8n, development dependencies, or unrelated services.
2. DOMPurify is dual-licensed as `MPL-2.0 OR Apache-2.0`. The current notice section contains the Apache-2.0 text. Preserve the full license section and make the selected alternative explicit in the internal license record. Upstream package metadata: https://github.com/cure53/DOMPurify
3. Bootstrap Icons 1.13.2 SVG artwork is separately embedded in the public tree. Its MIT notice and copyright statement are included in `THIRD_PARTY_NOTICES.txt`; keep this notice whenever the artwork is distributed.
4. BSD-3-Clause's no-endorsement condition is not satisfied merely by including the notice if the website separately uses the upstream author's name to endorse the portal. No such endorsement was established by this audit.
5. The notice file contains one section per package/version in the current lockfile-derived production closure. Deployment preflight should continue to verify this correspondence after every dependency update.

## Canonical license texts

- MIT: https://spdx.org/licenses/MIT.html
- Apache-2.0: https://spdx.org/licenses/Apache-2.0.html
- BSD-2-Clause: https://spdx.org/licenses/BSD-2-Clause
- BSD-3-Clause: https://spdx.org/licenses/BSD-3-Clause
- ISC: https://spdx.org/licenses/ISC.html
- MPL-2.0: https://spdx.org/licenses/MPL-2.0.html
- SPDX License List (versioned authoritative identifiers and text): https://spdx.org/licenses/

## Required follow-up before marking catalog entries reviewed

- Compare each captured package license file against its canonical text and package version.
- Verify all 95 package sections and the Bootstrap Icons artwork notice are included in the actual deployed artifact, not only the source tree.
- Confirm the DOMPurify alternative recorded in the license catalog; do not infer an internal approval status from this report.
- Review the license record in the compliance admin UI and store the reviewer, date, rationale, selected alternative, and evidence.
- Re-run strict license preflight and retain its report after any dependency or notice-file change.
