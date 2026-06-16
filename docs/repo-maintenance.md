# Repository Maintenance

Small index for docs, cleanup, and install bug-fix notes.

## Active Human Docs

| File | Purpose |
|------|---------|
| [README.md](../README.md) | Project intro |
| [docs/install-desktop.md](install-desktop.md) | Install and lifecycle |
| [docs/options.md](options.md) | Optional tiers |
| [SECURITY.md](../SECURITY.md) | Vulnerability reporting |
| [docs/security.md](security.md) | Operator hardening |
| [CONTRIBUTING.md](../CONTRIBUTING.md) | Contributor workflow |
| [docs/product-roadmap.md](product-roadmap.md) | Compact roadmap |
| [docs/scraper-cloudflare-and-proxy.md](scraper-cloudflare-and-proxy.md) | Scraper notes |
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
| 2026-06-15 | pending | Require setup-control proxy readiness during install/start/repair and clean stale Docker resources without `.env` |
| 2026-06-15 | pending | Repair localhost relays for IPv4 and IPv6 |
| 2026-06-15 | pending | Repo hygiene: `make check`, CodeQL, CODEOWNERS, issue templates, `.gitignore` cleanup |

## Legacy Config Aliases

Keep until a migration test proves zero usage:

| Alias | Canonical |
|-------|-----------|
| `FIRECRAWL_API_URL` | `SCRAPER_API_URL` |
| `FIRECRAWL_API_KEY` | `SCRAPER_API_KEY` |
