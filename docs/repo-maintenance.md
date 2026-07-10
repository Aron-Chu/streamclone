# Repository Maintenance

Small index for docs, cleanup, and install bug-fix notes.

## Active Human Docs

| File | Purpose |
|------|---------|
| [README.md](../README.md) | Project intro |
| [docs/install-desktop.md](install-desktop.md) | Install and lifecycle |
| [docs/options.md](options.md) | Optional tiers |
| [docs/ENVIRONMENT.md](ENVIRONMENT.md) | Compose profiles and env overlays |
| [docs/SERVICE_MAP.md](SERVICE_MAP.md) | Service boundaries and ports |
| [docs/TESTING.md](TESTING.md) | Test matrix and focused commands |
| [docs/MCP.md](MCP.md) | Local MCP setup and usage |
| [docs/CODEX.md](CODEX.md) | Codex MCP, skills mirror, config |
| [docs/workspace.md](workspace.md) | Two-repo layout, doc ownership, dev workflow |
| [docs/streampulse-product-boundary.md](streampulse-product-boundary.md) | What is *not* in public Streamclone (StreamPulse backend/ops) |
| Extension spec | [streamclone-pulse `docs/pulse-extension/`](https://github.com/Aron-Chu/streamclone-pulse/tree/master/docs/pulse-extension) |
| [SECURITY.md](../SECURITY.md) | Vulnerability reporting |
| [docs/security.md](security.md) | Operator hardening |
| [CONTRIBUTING.md](../CONTRIBUTING.md) | Contributor workflow |
| [docs/scraper-cloudflare-and-proxy.md](scraper-cloudflare-and-proxy.md) | Scraper notes |
| [docs/tiers-scraper-and-social-spread.md](tiers-scraper-and-social-spread.md) | Tier detachment, scraper coupling, proxies, Social spread |
| [docs/archive/scraping-archive/requirements.md](archive/scraping-archive/requirements.md) | Bulk scrape + Azure blob archive requirements (archived) |
| [docs/storage/README.md](storage/README.md) | Storage SoT index — Azure authoritative, R2 planned, VOD Library direction |
| [docs/storage/azure-to-r2-migration.md](storage/azure-to-r2-migration.md) | Azure → R2 migration audit (Phase 0.6 inventory, Phase 1 prep) |
| [docs/agents-streamclone-and-replayforge.md](agents-streamclone-and-replayforge.md) | Clip Studio / ReplayForge boundary |
| [docs/production-artifact-contract.md](production-artifact-contract.md) | GHCR image tags and ops deploy contract |

Agent docs live in `AGENTS.md`, `.kiro/steering/`, `.cursor/skills/streamclone/`, and `tools/*/README.md`.

## Cleanup Rules

- Link from README only if end users need it.
- Keep deep implementation guidance in steering docs, not public docs.
- Delete one-off review briefs instead of archiving them.
- Keep `SECURITY.md` short; put details in `docs/security.md`.
- Do not commit generated media frames, build output, `.env`, logs, or local probe files.

## GitHub Settings

Enable:

- secret scanning and push protection
- Dependabot security updates
- branch protection on `master`
- required CI and CodeQL checks

### Branch protection on `master` (exact)

Required status check names (must match job `name:` in `.github/workflows/ci.yml`):

- `Secret scan`
- `Full gate (make check)`
- `Code graph rebuild`
- `Core compose smoke`

Optional: add CodeQL job name from `.github/workflows/codeql.yml` after verifying in Actions UI.

```bash
gh api --method PUT \
  -H "Accept: application/vnd.github+json" \
  repos/Aron-Chu/streamclone/branches/master/protection \
  -f required_status_checks[strict]=true \
  -f required_status_checks[contexts][]='Secret scan' \
  -f required_status_checks[contexts][]='Full gate (make check)' \
  -f required_status_checks[contexts][]='Code graph rebuild' \
  -f required_status_checks[contexts][]='Core compose smoke' \
  -f enforce_admins=true \
  -f required_pull_request_reviews[required_approving_review_count]=1 \
  -f restrictions=null \
  -f allow_force_pushes=false \
  -f allow_deletions=false
```

Verify: `gh api repos/Aron-Chu/streamclone/branches/master/protection --jq '.required_status_checks.contexts'`

If API returns 403, document these settings manually in GitHub → Settings → Branches.

## Naming

| Name | Meaning |
|------|---------|
| Streamclone | Product and GitHub repo |
| This git checkout | Source tree for changes |
| `%USERPROFILE%\streamclone` | Release install, usually not git |

Bug fixes ship from this repo. Copying source into the release install only updates scripts/docs, not built Go/frontend images.

## Install Bug Fix Log

| Date | Commit | Summary |
|------|--------|---------|
| 2026-06-12 | `04b79c5` | Defer Docker cleanup on uninstall when Docker Desktop is offline |
| 2026-06-13 | `4bb1298` | Bootstrap from `%TEMP%` correctly and allow same-version reinstall |
| 2026-06-13 | `966c6b0` | Fetch bootstrap and script overlay by GitHub commit SHA |
| 2026-06-13 | `a67bd05` | Repair `deploy/Caddyfile.local-tunnel` when Docker creates a directory |
| 2026-06-15 | `fc6406c` | Pin release install asset version per tag |
| 2026-06-15 | `fc6406c` (assessed 2026-06-21) | Require setup-control proxy readiness during install/start/repair — `ensure-setup-control.ps1 -RequireProxy` in start/setup scripts |
| 2026-06-15 | `fc6406c` (assessed 2026-06-21) | Repair localhost relays for IPv4 and IPv6 — `ensure-localhost-relays.ps1` |
| 2026-06-15 | assessed 2026-06-21 | Repo hygiene (Phase 1): doc/router alignment (`AGENTS.md`, `SERVICE_MAP.md`, `docs/workspace.md`), `.gitignore` grafana PNG pattern, dead frontend shims removed, clipper profile refs cleaned — remaining: `make check`, CodeQL, CODEOWNERS, issue templates |
| 2026-06-17 | assessed 2026-06-21 | Install automation (partial): ReplayForge vs clipper cleanup done in Makefile/skills; Pulse Wire release defaults, release env merge on upgrade, warming UI — verify and close per item |
| 2026-06-19 | `fc6406c` (assessed 2026-06-21) | Uninstall uses same compose profiles as start/stop (feature flags from `.env`) on Windows and bash |
| 2026-07-09 | `2fb5385` | Core-only boundary lock: remove Analytics/ReplayForge from watch UI, install hints, clipper stub, and product-boundary gate; Desktop Start must use new release tag (not stale `%USERPROFILE%\streamclone` v0.3.0-rc8) |

## Legacy Config Aliases

Keep until a migration test proves zero usage:

| Alias | Canonical |
|-------|-----------|
| `FIRECRAWL_API_URL` | `SCRAPER_API_URL` |
| `FIRECRAWL_API_KEY` | `SCRAPER_API_KEY` |
