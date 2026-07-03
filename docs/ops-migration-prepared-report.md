# Ops migration prepared report — 2026-07-03

**Status: prepared / audited — not complete.** Live cutover and public PR merge are blocked until rescue validation passes and a release publishes the `migrate` GHCR image.

## Summary

Host-specific production ops are being moved from public **streamclone** into private **[streampulse-ops](https://github.com/Aron-Chu/streampulse-ops)**. Public repo retains app code, migrations, local dev compose, CI, and release image builds.

## Moved to streampulse-ops

- ~68 `scripts/bearhost*` → `archive/bearhost/`
- Active `scripts/streampulse-vps*` → `scripts/deploy/` (legacy paths quarantined under `archive/deploy-legacy/`)
- Production compose overlays + image-only `compose/production/`
- Env examples (`env/examples/production.env.example`)
- Operator docs → `docs/architecture/`, `archive/`
- Rollback runbook, smoke scripts, import manifest

## Stayed in streamclone

- `cmd/`, `internal/`, `frontend/`, `packages/`, `migrations/`
- Local compose: `docker-compose.yml`, `local-tunnel`, `release`, `prod`
- CI workflows, `Makefile` local targets
- `deploy/Dockerfile.migrate` + release workflow migrate image
- Public docs: install, ENVIRONMENT, production-artifact-contract

## Stubbed in streamclone

- `docs/site-links.md`, `docs/bearhost-production.md`, `docs/operator-secrets.md`
- Makefile bearhost/grafana targets → `scripts/ops-stub.sh`
- `AGENTS.md` routing → private streampulse-ops

## Open blockers (rescue pass)

| ID | Issue | Status |
|----|--------|--------|
| B1 | Release workflow `pull` missing `migrate` in single compose command | Fixed in rescue pass |
| B2 | Public working tree mixes ops migration + unrelated product deletions | Narrow `chore/ops-migration-only` branch required |
| B3 | Ops compose rendered config still had `build:` / scraper context | Slim `docker-compose.services.yml` (GHCR-only) |
| B4 | Legacy deploy scripts in active path (broken ROOT, rsync) | Quarantined to `archive/deploy-legacy/` |
| B5 | Makefile help / Windows `local-vps-only` referenced deleted BearHost scripts | Fixed in rescue pass |

## Re-validation commands

### streamclone

```bash
bash scripts/ci-bootstrap-env.sh
bash scripts/env-synthesize.sh core /tmp/test.env
make compose-config-check
rg "\.env\.dev" --glob '!docs/ops-migration*' --glob '!migration-baseline.md'
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

- **Migrate GHCR image** — requires next release tag push after `Dockerfile.migrate` merges.
- **Branch protection** — API enable failed (422); configure manually per `docs/repo-maintenance.md`.
- **Live production cutover** — operator must run `production-deploy.sh` on VPS with real `env/production.local.env`; evidence only in `streampulse-ops/docs/deployments/` after live deploy.
- **Migrations irreversible** — rollback runbook documents image-only rollback limits.

## Follow-up

1. Merge narrow `chore/ops-migration-only` PR in streamclone.
2. Tag streamclone release with migrate image; confirm GHCR has `migrate` at tag.
3. Operator live deploy + update `streampulse-ops/docs/deployments/`.
