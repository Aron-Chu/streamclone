# Ops migration prepared report — 2026-07-03

**Status: prepared / audited — not complete.** Live cutover and public PR merge are blocked until rescue validation passes and a release publishes the `migrate` GHCR image on GHCR.

## Summary

Host-specific production ops are being copied from public **streamclone** into private **[streampulse-ops](https://github.com/Aron-Chu/streampulse-ops)**. This public branch prepares migration (env synthesis, migrate image, stubs, CI) but does **not** remove legacy BearHost/VPS/Grafana files yet.

Public repo retains app code, migrations, local dev compose, CI, and release image builds.

## Copied / imported to streampulse-ops (private)

Ops artifacts have been **copied and imported** into private `streampulse-ops` (not deleted from public in this PR):

- ~68 `scripts/bearhost*` → `archive/bearhost/`
- Legacy `scripts/streampulse-vps*` → `archive/deploy-legacy/` (active path: `production-deploy.sh`)
- Production compose overlays + GHCR-only `compose/production/docker-compose.services.yml`
- Env examples (`env/examples/production.env.example`)
- Operator docs → `docs/architecture/`, `archive/`
- Rollback runbook, smoke scripts, import manifest

## Public branch scope (this PR)

- Routes hosted ops entrypoints to stubs (`scripts/ops-stub.sh`, doc pointers)
- Adds `scripts/env-synthesize.sh`, `deploy/env/profile-dev.env`, `deploy/Dockerfile.migrate`
- Fixes release workflow migrate pull + Makefile help / Windows `local-vps-only`
- Adds migration docs and `docs/production-artifact-contract.md`

## Public removal deferred

Legacy BearHost/VPS/Grafana scripts, compose overlays, and operator docs **remain in the public repo** on `origin/master` until a follow-up PR after private ops validation and migrate image release. This narrow PR does not bundle those deletions.

## Stayed in streamclone

- `cmd/`, `internal/`, `frontend/`, `packages/`, `migrations/`
- Local compose: `docker-compose.yml`, `local-tunnel`, `release`, `prod`
- CI workflows, `Makefile` local targets
- `deploy/Dockerfile.migrate` + release workflow migrate image
- Public docs: install, ENVIRONMENT, production-artifact-contract

## Stubbed in streamclone (this branch)

- `docs/site-links.md`, `docs/bearhost-production.md`, `docs/operator-secrets.md`
- Makefile bearhost/grafana targets → `scripts/ops-stub.sh` (help points to private `streampulse-ops`)
- `AGENTS.md` routing → private streampulse-ops

## Open blockers (rescue pass)

| ID | Issue | Status |
|----|--------|--------|
| B1 | Release workflow `pull` missing `migrate` in single compose command | Fixed |
| B2 | Public working tree mixes ops migration + unrelated product deletions | Narrow `chore/ops-migration-only` branch |
| B3 | Ops compose rendered config had `build:` / scraper context | Fixed in streampulse-ops (`docker-compose.services.yml`) |
| B4 | Legacy deploy scripts in active path | Quarantined to `archive/deploy-legacy/` |
| B5 | Makefile help / Windows `local-vps-only` referenced deleted BearHost scripts | Fixed |

## Re-validation commands

### streamclone

```bash
bash scripts/ci-bootstrap-env.sh
bash scripts/env-synthesize.sh core /tmp/test.env
make compose-config-check
git grep -n '\.env\.dev' -- scripts deploy .github .pre-commit-config.yaml docs \
  ':!docs/ops-migration-plan.md' ':!docs/ops-migration-manifest.md' ':!migration-baseline.md'
```

### streampulse-ops

```bash
IMAGE_TAG=v0.2.10 docker compose \
  --env-file env/examples/production.env.example \
  -f compose/production/docker-compose.yml config
bash scripts/smoke/validate-production-compose.sh
bash scripts/smoke/validate-production-env.sh --example env/examples/production.env.example
bash scripts/smoke/production-smoke.sh --dry-run
IMAGE_TAG=v0.2.10 DRY_RUN=1 bash scripts/deploy/production-deploy.sh
```

## Remaining risks (before cutover)

- **Migrate GHCR image** — requires next release tag push after `Dockerfile.migrate` merges; **cutover deferred** until `ghcr.io/aron-chu/streamclone/migrate:${TAG}` exists.
- **Branch protection** — API enable failed (422); configure manually per `docs/repo-maintenance.md`.
- **Live production cutover** — operator must run `production-deploy.sh` on VPS with real `env/production.local.env`; evidence only in `streampulse-ops/docs/deployments/` after live deploy.
- **Migrations irreversible** — rollback runbook documents image-only rollback limits.

## Follow-up

1. Merge narrow `chore/ops-migration-only` PR in streamclone.
2. Tag streamclone release with migrate image; confirm GHCR has `migrate` at tag.
3. Follow-up public PR: remove legacy ops files after private ops soak.
4. Operator live deploy + update `streampulse-ops/docs/deployments/`.
