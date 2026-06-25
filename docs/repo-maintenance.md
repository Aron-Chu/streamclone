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
| [docs/pulse-extension/](pulse-extension/) | Redirect stubs → [streamclone-pulse extension spec](https://github.com/Aron-Chu/streamclone-pulse/tree/master/docs/pulse-extension) |
| [packages/pulse-core/](../packages/pulse-core/) | Shared scoring/types for frontend + extension |
| [SECURITY.md](../SECURITY.md) | Vulnerability reporting |
| [docs/security.md](security.md) | Operator hardening |
| [CONTRIBUTING.md](../CONTRIBUTING.md) | Contributor workflow |
| [docs/scraper-cloudflare-and-proxy.md](scraper-cloudflare-and-proxy.md) | Scraper notes |
| [docs/tiers-scraper-and-social-spread.md](tiers-scraper-and-social-spread.md) | Tier detachment, scraper coupling, proxies, Social spread |
| [docs/scraping-archive/requirements.md](scraping-archive/requirements.md) | Bulk scrape + Azure blob archive requirements |
| [docs/storage/README.md](storage/README.md) | Storage SoT index — Azure authoritative, R2 planned, VOD Library direction |
| [docs/storage/azure-to-r2-migration.md](storage/azure-to-r2-migration.md) | Azure → R2 migration audit (Phase 0.6 inventory, Phase 1 prep) |
| [docs/agents-streamclone-and-replayforge.md](agents-streamclone-and-replayforge.md) | Clip Studio / ReplayForge boundary |
| [deploy/FREE_DEPLOYMENT.md](../deploy/FREE_DEPLOYMENT.md) | Public VM notes |

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
| 2026-06-19 | `fc6406c` (assessed 2026-06-21) | Uninstall uses same compose profiles as start/stop (`pulse-wire`, feature flags from `.env`) on Windows and bash |

## Legacy Config Aliases

Keep until a migration test proves zero usage:

| Alias | Canonical |
|-------|-----------|
| `FIRECRAWL_API_URL` | `SCRAPER_API_URL` |
| `FIRECRAWL_API_KEY` | `SCRAPER_API_KEY` |
