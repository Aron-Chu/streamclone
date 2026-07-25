# Public ref contamination report (read-only) — 2026-07-25

**Repository:** Aron-Chu/streamclone  
**Default branch tip scanned:** current PR branch / master base `40f015d`  
**Action taken on default branch:** archive agent-plans containing topology tokens were
replaced with non-sensitive stubs. History was **not** rewritten.

## Current-tree status

After this PR, `scripts/ci-public-topology-scan.sh` must pass on every tracked
file (including `docs/archive/**`). Soft-skip is removed.

## Historical risk (owner action required)

Git history and non-default refs may still contain production topology that
previously lived under `docs/archive/agent-plans/*` and related docs.

Recommended owner-approved follow-ups (do **not** execute without approval):

1. Inventory all refs: `git for-each-ref --format='%(refname)'`
2. Search history for former production host octets / private ops path tokens
   using operator-known patterns (do not re-document literal addresses here).
3. If contamination remains on published tags/branches, plan a coordinated
   `git filter-repo` / branch deletion with private ops backup first.
4. Do not force-push rewritten history without a written owner plan.

## Release asset OAuth risk (read-only)

Published desktop archives (`streamclone-*-windows.zip`, `streamclone-*.tar.gz`)
were produced while `package-release.sh` could embed `oauth-bundle.env` when
GitHub Actions secrets were configured.

| Observation | Detail |
|-------------|--------|
| Asset classes present | `streamclone-*-windows.zip`, `streamclone-*.tar.gz`, installer cmd/exe |
| Full binary download | Not performed in this audit |
| Suspected exposure | **Cannot exclude** — any release built with both Twitch OAuth secrets set likely contains `deploy/env/oauth-bundle.env` |
| Owner action | **Rotate** any Twitch OAuth client secret that may have been present in Actions secrets used for those releases; treat distributed archives as untrusted for secret material |
| Not done | No release deletion, no credential rotation, no asset rewrite |

## Non-claims

- No history rewrite
- No branch/tag deletion
- No secret rotation performed
- No production deploy
