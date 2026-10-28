# Source baseline and traceability

- Project: `portal-static-assets`
- Initial source repository: `https://github.com/lycupay-debug/personal-portal`
- Selected baseline commit: `7d36218343da4a74a29f788b219231e65f3264f2`
- Baseline branch at selection: `refactor/portal-app-table-composable-20261028`
- Baseline worktree at selection: `/opt/personal-portal-refactor-20261028`
- New independent working directory: `/opt/portal-static-assets`

The initial tree is based on the selected commit. The original Git history is retained in this clone. Project experiments and commits must remain separate from the production directory `/opt/personal-portal`. Do not deploy or restart production services as part of this project without explicit authorization.

## Release safety

`npm run package:static-assets` creates a local release candidate only. The CI workflow is manual and publishes a workflow artifact, not a live site. A real deployment requires a separately approved destination and URL strategy because current CSS references include root-relative `/icons/...` paths.
