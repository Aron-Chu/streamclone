# Hosted production ops (private)

Streamclone **application source**, migrations, local dev compose, CI, and GHCR image builds live in this public repository.

**Hosted production execution** (hosted-production-vps deploy, secrets, smoke, rollback evidence) lives in private **streampulse-ops**.

## Public contract

- [production-artifact-contract.md](production-artifact-contract.md) — `IMAGE_TAG`, GHCR images, migrate invariant
- [ops-migration-manifest.md](ops-migration-manifest.md) — what stays public vs private

## Operator entrypoints (private)

| Task | Location |
|------|----------|
| Production deploy | `streampulse-ops/scripts/deploy/production-deploy.sh` |
| Post-deploy smoke | `streampulse-ops/scripts/smoke/production-smoke.sh` |
| Compose validation | `streampulse-ops/scripts/smoke/validate-production-*.sh` |
| Deployment evidence | `streampulse-ops/docs/deployments/` |
| BearHost rollback archive | `streampulse-ops/archive/bearhost/` |

## Local / self-hosted

Use `make up`, `make compose-config-check`, and [ENVIRONMENT.md](ENVIRONMENT.md) — not production VPS runbooks in this repo.

## BearHost (historical)

Rollback-only material was archived in private ops during the 2026-07 boundary migration. See stub [bearhost-production.md](bearhost-production.md).
