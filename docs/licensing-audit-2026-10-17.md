# Personal Portal typography and licensing audit

Date: 2026-10-17
Scope: `/opt/personal-portal` frontend source, dependency lockfile, public assets, deployed portal releases, and a read-only inventory of adjacent server assets.

## Implemented in the portal

- Default text font stack is `system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`; mono text uses the platform monospace stack.
- All 359 `font-size` declarations found in the 7 CSS/Vue source files have been migrated to CSS custom properties in `src/theme.css`; static audit found no remaining literal `font-size` declarations and no unresolved `--font-size-*` variables.
- Small sizes originally below the policy threshold were normalized to at least 14px outside card/tile selectors and at least 12px in card/tile selectors. This is a source-level audit; browser-computed values and responsive overflow still need a real browser engine to verify.
- Removed the portal's bundled `bootstrap-icons.woff2` and its direct `bootstrap-icons` package dependency. The 37 icon names used by the current source now reference local SVG assets instead.
- `public/THIRD_PARTY_NOTICES.txt` contains the Bootstrap Icons MIT license text and notices for installed production dependency packages.

## Third-party license findings

- `bootstrap-icons@1.13.2`: MIT. SVG artwork remains attributed and the MIT notice is included in `public/THIRD_PARTY_NOTICES.txt`.
- `vanilla-calendar-pro@3.4.0`: installed package metadata and `LICENSE` identify MIT; the package name alone is not evidence of a paid or non-commercial restriction.
- `bootstrap@6.0.0-alpha.1`: MIT metadata and license. The alpha version is a stability consideration, not evidence of a commercial-use restriction.
- The installed production dependency closure was enumerated from `package-lock.json` and package metadata; third-party notices are provided as a compliance aid, not a legal opinion. The inventory cannot establish the provenance or rights of user-uploaded content or external content fetched at runtime.

## Server-level inventory and exceptions

- The initial scan found 74 font files under `/usr/local/www/personal-portal/releases`, including duplicated Bootstrap Icons fonts in historical releases and KaTeX fonts in an old Vditor release. These are portal release artifacts; font files are removed from release directories after the new release is deployed. Historical rollback releases will therefore no longer preserve their original font-based icon/math rendering.
- Five font assets exist in `/opt/n8n-test/n8n-src-2.42.6/packages/frontend/@n8n/design-system/assets/fonts/` (InterVariable variants, CommitMonoVariable, and Element icon font formats). They are part of the installed n8n application source and are not portal assets. Deleting them blindly would break or degrade the n8n editor's typography and icon glyphs; they are preserved pending an application-specific replacement plan and license review.
- 212 font files were found under `/usr/local/share/doc/rust/`, within Rust documentation assets. They are not served by the personal portal and belong to installed local documentation. They are preserved rather than modifying system package contents without a dependency/license-based reason.
- The initial scan did not find font files in `/usr/local/etc`. Font files under operating-system/application dependency directories are not automatically evidence of a commercial license violation.

## Significant licensing issue requiring separate review

`/opt/n8n-test/n8n-src-2.42.6/LICENSE.md` states that the source tree is under the Sustainable Use License 1.0 except for exclusions, and limits use to internal business purposes or non-commercial/personal use; it also says `.ee` files require a valid n8n Enterprise license. A read-only filename scan found 205 `.ee`-named or `.ee`-directory source files in that source tree. This does not by itself establish that the running deployment violates the license. The actual use case, the deployed runtime artifact, any applicable enterprise agreement, and whether n8n is provided to external customers need to be checked before commercializing n8n-powered functionality.

## Verification limits

- `npm run build` and `npm test` are run as part of this change.
- The FreeBSD host has Playwright libraries but no browser executable found in the scanned paths, so computed-style, screenshot, and viewport-overflow checks cannot be claimed as completed. HTTP checks verify server responses only, not rendered layout.
