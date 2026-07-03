# Ops migration baseline — 2026-07-03

Local audit for `Aron-Chu/streamclone` → private `streampulse-ops` boundary migration.

## Git context

| Item | Value |
|------|-------|
| Checkout | `C:\Users\Aron\twitch-7tv-clone` |
| Remote | `origin` → `https://github.com/Aron-Chu/streamclone.git` |
| Branch | `master` |
| Visibility | Public |
| Branch protection | None (404) |
| Latest stable release | `v0.2.10` |
| Latest prerelease | `v0.3.0-rc5` |

## Production state

| Host | Role |
|------|------|
| streampulse-vps (`23.173.152.156`) | Active production SoT |
| BearHost (`141.11.243.103`) | Rollback-only until soak passes |

Deploy today: rsync + `bearhost-build` overlay; optional `STREAMPULSE_USE_RELEASE_IMAGES=1` + `IMAGE_TAG`.

## CI (run 28636434856, master)

| Job | Result |
|-----|--------|
| Secret scan | pass |
| Full gate (`make check`) | pass |
| Code graph rebuild | **fail** |
| Core compose smoke | **fail** (health wait) |

## `.env.dev`

Exists locally on some machines, **not tracked**. CI and `chore/ops-migration-only` use `scripts/ci-bootstrap-env.sh` → `.env.example` and `scripts/env-synthesize.sh` (tracked `deploy/env/profile-dev.env`). Makefile `env` target and release paths no longer require `.env.dev` on the migration branch.

---

## Dirty tree triage (ops-relevant)

| path | state | needed for migration? | action |
|------|-------|----------------------|--------|
| `.github/workflows/hub-health-monitor.yml` | untracked | no (public API probe) | commit-before-migration, **stay** in streamclone |
| `scripts/streampulse-vps-cron-install.sh` | untracked | yes | copy-to-ops |
| `scripts/streampulse-vps-pg-backup.sh` | untracked | yes | copy-to-ops |
| `scripts/streampulse-vps-r2-corpus-cutover.sh` | untracked | yes | copy-to-ops |
| `scripts/streampulse-vps-top500-scale.sh` | untracked | yes | copy-to-ops |
| `scripts/bearhost-worker.sh` | untracked | rollback only | archive/bearhost |
| `deploy/docker-compose.bearhost-worker.yml` | untracked | rollback only | archive/bearhost |
| `deploy/env/profile-bearhost-worker.env.example` | untracked | rollback only | archive/bearhost |
| `deploy/env/profile-hosted-data-mcp.env.example` | untracked | yes | copy-to-ops |
| `deploy/env/profile-streampulse-vps-r2-corpus.env.example` | untracked | yes | copy-to-ops |
| `scripts/hosted-data-mcp.sh` | untracked | yes | copy-to-ops |
| `scripts/hosted-data-tunnel.sh` | untracked | yes | copy-to-ops |
| `deploy/alerts/top500-hosted.proposal.yml` | untracked | no | archive |
| `docs/archive/bearhost-rollback-artifacts.md` | untracked | yes | copy-to-ops archive |
| `scripts/test-streampulse-vps-production-compose.sh` | tracked (if present) | yes | copy-to-ops |
| `.claude/`, duplicate skill mirrors | untracked | no | ignore |
| All tracked `scripts/bearhost*` (68 files) | tracked | rollback/archive | copy-to-ops archive/bearhost |
| All tracked `scripts/streampulse-vps*` (6 files) | tracked | yes | copy-to-ops active |
| `deploy/docker-compose.streampulse-vps-production.yml` | tracked | yes | copy-to-ops active (adapt image-only) |
| BearHost compose overlays | tracked | rollback only | archive/bearhost |

**Rule:** Do not copy untracked files until classified above.
