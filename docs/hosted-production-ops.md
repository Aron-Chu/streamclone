# Hosted production ops (private)

Streamclone **application source**, migrations, local dev compose, CI, and GHCR **source** image builds live in this public repository.

**Hosted production execution** (deploy, secrets, smoke, rollback evidence) lives in private **streampulse-ops** — never add that checkout to public multi-root workspaces.

> **Confused about tags?** Read **[ops-migration-truth-table.md](ops-migration-truth-table.md)** before changing deploy or extension health UX.

## Public contract

- [production-artifact-contract.md](production-artifact-contract.md) — source-build: `IMAGE_TAG`, GHCR source images, migrate invariant
- [production-promotion-contract.md](production-promotion-contract.md) — hosted promotion: digest promotion, target `streampulse/*` namespace
- [ops-migration-manifest.md](ops-migration-manifest.md) — what stays public vs private
- [ops-migration-truth-table.md](ops-migration-truth-table.md) — tags vs private ops FAQ

## Operator entrypoints (private)

| Task | Location |
|------|----------|
| Production deploy | private `streampulse-ops/scripts/deploy/production-deploy.sh` |
| Post-deploy smoke | private `streampulse-ops/scripts/smoke/production-smoke.sh` |
| Deployment evidence | private `streampulse-ops/docs/deployments/` |
| Legacy rollback archive | private `streampulse-ops/archive/bearhost/` |

## Public API probes (safe from this repo)

```bash
curl -fsS https://api.streampulse.stream/v1/extension/health
bash scripts/hosted-launch-probes.sh
```

SSH, internal ops routes, and VPS shell checks belong in **private streampulse-ops** only.

## Local / self-hosted

Use `make up`, `make compose-config-check`, and [ENVIRONMENT.md](ENVIRONMENT.md).

## Legacy rollback host

Rollback-only material was archived in private ops during the 2026-07 boundary migration. See stub [bearhost-production.md](bearhost-production.md).
