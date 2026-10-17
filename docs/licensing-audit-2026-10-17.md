# Personal Portal typography and licensing audit

Date: 2026-10-17
Scope: `/opt/personal-portal` frontend source, dependency lockfile, public assets, deployed portal releases, and a read-only inventory of adjacent server assets.

## Implemented in the portal

- Default text font stack is `system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`; mono text uses the platform monospace stack.
- All 361 `font-size` declarations found in the 7 CSS/Vue source files have been migrated to CSS custom properties in `src/theme.css`; 11 font-size literals embedded in `font` shorthand declarations were also migrated to size tokens. Bootstrap compact utility classes are constrained by global floor rules that reference the same central variables. Static audit found no remaining literal `font-size` declarations and no unresolved `--font-size-*` variables.
- Small sizes originally below the policy threshold were normalized to at least 14px outside card/tile selectors and at least 12px in card/tile selectors. This is a source-level audit; browser-computed values and responsive overflow still need a real browser engine to verify.
- Removed the portal's bundled `bootstrap-icons.woff2` and its direct `bootstrap-icons` package dependency. The 37 icon names used by the current source now reference local SVG assets instead.
- `public/THIRD_PARTY_NOTICES.txt` contains the Bootstrap Icons MIT license text and notices for installed production dependency packages.

## Third-party license findings

- `bootstrap-icons@1.13.2`: MIT. SVG artwork remains attributed and the MIT notice is included in `public/THIRD_PARTY_NOTICES.txt`.
- `vanilla-calendar-pro@3.4.0`: installed package metadata and `LICENSE` identify MIT; the package name alone is not evidence of a paid or non-commercial restriction.
- `bootstrap@6.0.0-alpha.1`: MIT metadata and license. The alpha version is a stability consideration, not evidence of a commercial-use restriction.
- The installed production dependency closure was enumerated from `package-lock.json` and package metadata; third-party notices are provided as a compliance aid, not a legal opinion. The inventory cannot establish the provenance or rights of user-uploaded content or external content fetched at runtime.

## Server-level inventory and exceptions

- The initial scan found 74 font files under `/usr/local/www/personal-portal/releases`, including duplicated Bootstrap Icons fonts in historical releases and KaTeX fonts in an old Vditor release. All 74 were removed from portal release directories after deployment. Historical rollback releases will therefore no longer preserve their original font-based icon/math rendering.
- The broader scan found 111 font files under `/opt`: 64 in `/opt/n8n-test/node_modules/n8n-editor-ui/dist/assets/`, 14 in `pdfjs-dist/standard_fonts`, 12 in `js-md4/doc/fonts`, 10 under `@n8n`, 5 in the n8n source tree, and 6 in Playwright tooling (counts include some duplicated font formats/assets). The n8n editor assets include Inter, Element icon fonts, and KaTeX math fonts; deleting them blindly would degrade typography, icons, and math rendering in the running n8n editor. PDF.js standard fonts may be required for PDF rendering. These runtime assets are preserved pending an application-specific replacement plan and license review. Documentation/test-only font assets under `js-md4/doc/fonts`, Playwright inspector tooling, Rust documentation (`/usr/local/share/doc/rust`), temporary Bootstrap Icons extraction files, and the unused FreeBSD example Type 1 font were removed. The source-checkout copy of n8n design-system fonts is preserved to avoid mutating the n8n source tree.
- 212 font files were found under `/usr/local/share/doc/rust/`, within Rust documentation assets. They are not served by the personal portal and belong to installed local documentation. They are preserved rather than modifying system package contents without a dependency/license-based reason.
- The initial scan did not find font files in `/usr/local/etc`. Font files under operating-system/application dependency directories are not automatically evidence of a commercial license violation. A full-root scan after cleanup found 93 font-extension files remaining: 88 under the installed n8n dependency tree and 5 in the n8n source checkout. The 88 dependency assets include the running n8n editor UI fonts (Inter, Element icons, and KaTeX math fonts) and PDF.js standard fonts. Deleting them without replacing their consumers would degrade or break editor typography, icon glyphs, math rendering, or PDF rendering. They are retained as an explicit operational exception pending an application-specific replacement plan.

## Broader installed dependency audit

A recursive metadata scan across the installed n8n and portal-related Node.js dependency trees identified 1,899 package instances. The majority declare common permissive licenses (MIT, Apache-2.0, ISC, BSD). Review candidates include 46 packages whose metadata delegates to a local `LICENSE.md`, 18 packages with missing license metadata (some still contain a license file), four packages marked LGPL-3.0-or-later (`@zenfs/core`, `@zenfs/streams`, `memium`, `utilium`), and two packages marked `LicenseRef-n8n-enterprise` (`@n8n_io/ai-assistant-sdk`, `@n8n_io/license-sdk`). These counts are package instances, not unique products, and are not a finding of infringement. `jszip` and `node-forge` declare dual-license options including MIT/BSD alternatives; their exact dependency use should be confirmed before any distribution. The two `LicenseRef-n8n-enterprise` packages require checking against the installed n8n edition and any applicable agreement before using the associated enterprise-only capabilities commercially.

## Significant licensing issue requiring separate review

`/opt/n8n-test/n8n-src-2.42.6/LICENSE.md` states that the source tree is under the Sustainable Use License 1.0 except for exclusions, and limits use to internal business purposes or non-commercial/personal use; it also says `.ee` files require a valid n8n Enterprise license. A read-only filename scan found 205 `.ee`-named or `.ee`-directory source files in that source tree. This does not by itself establish that the running deployment violates the license. The actual use case, the deployed runtime artifact, any applicable enterprise agreement, and whether n8n is provided to external customers need to be checked before commercializing n8n-powered functionality.

## Verification limits

- `npm run build` and `npm test` are run as part of this change.
- The FreeBSD host has Playwright libraries but no browser executable found in the scanned paths, so computed-style, screenshot, and viewport-overflow checks cannot be claimed as completed. HTTP checks verify server responses only, not rendered layout.

## Detailed LGPL trace

The four LGPL packages were traced separately; see [`lgpl-dependency-trace-2026-10-17.md`](lgpl-dependency-trace-2026-10-17.md) for their parent chain, portal build checks, isolated module-load test, workflow scan, and current distribution assessment.
