# Ops migration file manifest

Classification for `streamclone` → `streampulse-ops` boundary migration.
**Copy first, delete from public repo only after validation** (see `docs/ops-migration-plan.md`).

---

## 1. Stay in streamclone

### Application code
- `cmd/**`, `internal/**`, `frontend/**`, `packages/**`, `migrations/**`
- `go.mod`, `go.sum`, `VERSION`

### Local / generic compose
- `deploy/docker-compose.yml`
- `deploy/docker-compose.local-tunnel.yml`
- `deploy/docker-compose.release.yml`
- `deploy/docker-compose.prod.yml`
- `deploy/docker-compose.laptopworker-dev.yml`
- `deploy/docker-compose.frontend-source.yml`
- `deploy/docker-compose.scraper-source.yml`
- `deploy/Caddyfile`, `deploy/Caddyfile.local-tunnel`
- `deploy/Dockerfile`, `deploy/Dockerfile.*` (app images)

### Local env profiles
- `deploy/env/profile-core.env`
- `deploy/env/profile-scraper.env`
- `deploy/env/profile-full.env`
- `deploy/env/profile-laptopworker-dev.env`
- `deploy/env/profile-dev.env` (new, tracked dev defaults)
- `deploy/env/profile-local-hybrid.env`, `profile-local-vps-only.env`
- `deploy/env/profile-bronze-*.env` (dev smoke)

### CI / release
- `.github/workflows/ci.yml`
- `.github/workflows/codeql.yml`
- `.github/workflows/release-images.yml`
- `.github/workflows/hub-health-monitor.yml` (public API probe)

### Scripts (local dev / release)
- `scripts/ci-bootstrap-env.sh`, `scripts/env-synthesize.sh`
- `scripts/lib/env.sh`, `scripts/lib/env.ps1`
- `scripts/smoke-core.sh`, `scripts/compose-down.sh`, `scripts/bootstrap.*`
- `scripts/package-release.sh` (after `.env.dev` removal)
- `scripts/local-vps-only.sh` (dev hybrid handoff)

### Public docs
- `README.md`, `CONTRIBUTING.md`, `SECURITY.md`
- `docs/install-desktop.md`, `docs/options.md`, `docs/ENVIRONMENT.md`
- `docs/SERVICE_MAP.md`, `docs/TESTING.md`, `docs/security.md`
- `docs/production-artifact-contract.md`
- `deploy/FREE_DEPLOYMENT.md`
- `AGENTS.md` (routing updated to point ops to private repo)

---

## 2. Move to streampulse-ops

### Active production (streampulse-vps)
| streamclone path | ops destination |
|------------------|-----------------|
| `deploy/docker-compose.streampulse-vps-production.yml` | `compose/production/overlays/streampulse-vps-production.yml` |
| `deploy/docker-compose.streampulse-vps-corpus.yml` | `compose/archive/streampulse-vps-corpus.yml` |
| `deploy/docker-compose.streampulse-vps-production-tailnet.yml` | `compose/archive/` |
| `deploy/env/profile-streampulse-vps-production.env.example` | `env/examples/production.env.example` |
| `deploy/env/profile-streampulse-vps-r2-corpus.env.example` | `env/examples/r2-corpus.env.example` |
| `deploy/env/profile-hosted-data-mcp.env.example` | `env/examples/hosted-data-mcp.env.example` |
| `scripts/streampulse-vps-production-deploy.sh` | `scripts/deploy/production-deploy.sh` (rewrite image-first) |
| `scripts/streampulse-vps-production-restore.sh` | `scripts/restore/production-restore.sh` |
| `scripts/streampulse-vps-production-backup-bearhost.sh` | `scripts/backup/bearhost-backup.sh` |
| `scripts/streampulse-vps-corpus-deploy.sh` | `scripts/deploy/corpus-deploy.sh` |
| `scripts/streampulse-vps-cron-install.sh` | `scripts/deploy/cron-install.sh` |
| `scripts/streampulse-vps-pg-backup.sh` | `scripts/backup/pg-backup.sh` |
| `scripts/streampulse-vps-r2-corpus-cutover.sh` | `scripts/deploy/r2-corpus-cutover.sh` |
| `scripts/streampulse-vps-top500-scale.sh` | `scripts/deploy/top500-scale.sh` |
| `scripts/streampulse-vps-production-tailnet-db.sh` | `scripts/archive/` |
| `scripts/lib/streampulse-vps-production-compose.sh` | `scripts/lib/production-compose.sh` (no BearHost in default) |
| `scripts/lib/deploy-rsync.sh` | `archive/break-glass/deploy-rsync.sh` |
| `scripts/hosted-data-mcp.sh` | `scripts/observability/hosted-data-mcp.sh` |
| `scripts/hosted-data-tunnel.sh` | `scripts/observability/hosted-data-tunnel.sh` |
| `scripts/test-streampulse-vps-production-compose.sh` | `scripts/smoke/compose-config-check.sh` |
| `docs/agent-notes/streampulse-vps-production-migration-2026-07-02.md` | `docs/architecture/` |
| `docs/agent-notes/streampulse-vps-unblock-audit-2026-07-02.md` | `docs/architecture/` |
| `docs/agent-notes/dual-vps-production-2026-07-02.md` | `docs/architecture/` |

### Image-only production compose (new in ops)
- `compose/production/docker-compose.yml` — GHCR images + `IMAGE_TAG` + migrate image
- `compose/production/docker-compose.release.yml` — adapted from streamclone release overlay

---

## 3. Archive in streampulse-ops

### BearHost rollback (`archive/bearhost/`)
All tracked paths:
- `deploy/docker-compose.bearhost-build.yml`
- `deploy/docker-compose.bearhost-prod.yml`
- `deploy/docker-compose.bearhost-pulse.yml`
- `deploy/docker-compose.bearhost-pulse-staging.yml`
- `deploy/docker-compose.bearhost-corpus-remote-worker.yml`
- `deploy/docker-compose.bearhost-worker.yml` (untracked)
- `deploy/env/profile-bearhost-*.env*`
- `deploy/Caddyfile.bearhost`
- All `scripts/bearhost*` (68 scripts)
- `docs/bearhost-production.md` (full copy before public stub)
- `docs/archive/bearhost-rollback-artifacts.md`

### Azure / proposals (`archive/azure/`)
- `deploy/docker-compose.azure-archive-plane.yml`
- `deploy/docker-compose.azure-scraper.yml`
- `deploy/env/profile-azure-*.env`
- `scripts/storage/corpus-azure-*`

### Launch notes (`archive/launch-notes/`)
- `docs/launch-readiness/*`
- `docs/agent-notes/launch-readiness-2026-07-02.md`

### Break-glass (`archive/break-glass/`)
- Rsync-first deploy scripts
- `bearhost-build` compose overlay
- Checkout-mounted `migrate/migrate` flow

---

## 4. Replace with public stub

After private copy validates:

| path | stub behavior |
|------|---------------|
| `docs/site-links.md` | Local + GitHub links only; no production IPs |
| `docs/bearhost-production.md` | Pointer to private `streampulse-ops` |
| `docs/operator-secrets.md` | Generic secret types only |
| `Makefile` bearhost/grafana targets | Echo redirect message |
| `docs/agent-notes/*production*` | Stub or redirect |

Stub text:
> Hosted production ops are maintained in private `streampulse-ops`. This public repo contains only local/self-hosted examples. Do not put production secrets or host-specific runbooks here.

---

## 5. Delete after validation

**Status:** Prepared in draft PR `chore/public-ops-cleanup-prep` — **not merged** until soak stable.

Duplicates in `streamclone` listed in sections 2–3 removed in that PR after:
- Ops copy committed and `import-manifest.md` marked active
- Live cutover evidence in `streampulse-ops/docs/deployments/2026-07-03-v0.3.0-rc7.md`
- Inventory: [public-ops-cleanup-inventory.md](public-ops-cleanup-inventory.md)

---

## 6. Needs human review

| path | reason |
|------|--------|
| `deploy/docker-compose.laptopworker-dev.yml` | Tailnet local dev — likely stay public |
| `docs/laptopworker-dev.md` | Same |
| `scripts/local-vps-only.sh` | Dev hybrid — stay, update doc links |
| `deploy/Caddyfile` | Shared base — stay |
| `deploy/alerts/top500-hosted.proposal.yml` | Proposal only |
| `.github/workflows/hub-health-monitor.yml` | Uses public API URL — stay in streamclone |
