# Hosted production ops (private)

Streamclone **application source**, migrations, local dev compose, CI, and GHCR **source** image builds live in this public repository.

**Hosted production execution** (hosted-production-vps deploy, secrets, smoke, rollback evidence) lives in private **streampulse-ops** — never add that checkout to public multi-root workspaces.

## Public contract

- [production-artifact-contract.md](production-artifact-contract.md) — source-build: `IMAGE_TAG`, GHCR source images, migrate invariant
- [production-promotion-contract.md](production-promotion-contract.md) — hosted promotion: digest promotion, target `streampulse/*` namespace
- Sibling [streamclone-image-exit-audit-2026-07.md](../../streamclone-pulse/docs/pulse-extension/evidence/streamclone-image-exit-audit-2026-07.md) — active migration spec (pre-cutover)
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
