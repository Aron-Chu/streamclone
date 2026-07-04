# Public ops cleanup inventory (prep PR)

**Branch:** `chore/public-ops-cleanup-prep`
**Base:** `origin/master` (`1557d75`)
**Private mirror:** `Aron-Chu/streampulse-ops` (`0840cd2`)

This PR removes host-specific production ops from public **streamclone** after private import validation. **Do not merge** until post-cutover soak is stable and explicitly approved.

## Removed from public streamclone → streampulse-ops mapping

| Category | Public paths removed | Private destination | Status |
|----------|---------------------|-------------------|--------|
| BearHost scripts | `scripts/bearhost*`, `scripts/lib/bearhost*` | `archive/bearhost/` | archived |
| VPS deploy scripts | `scripts/streampulse-vps*`, `scripts/lib/streampulse-vps-production-compose.sh` | `scripts/deploy/`, `archive/deploy-legacy/` | active + archived |
| BearHost compose | `deploy/docker-compose.bearhost*` | `archive/bearhost/` | archived |
| VPS compose overlays | `deploy/docker-compose.streampulse-vps*` | `compose/production/overlays/`, `compose/archive/` | active |
| VPS env example | `deploy/env/profile-streampulse-vps-production.env.example` | `env/examples/production.env.example` | active |
| BearHost env | `deploy/env/profile-bearhost*` | `archive/bearhost/` | archived |
| Grafana/Prometheus | `deploy/grafana/`, `deploy/prometheus/` | obsolete (ops UI strip 2026-07) | documented removed |
| Helm | `scripts/helm-*`, `charts/pulse/` | obsolete | documented removed |
| Observability compose | `deploy/docker-compose.observability*.yml` | obsolete | documented removed |
| Host runbooks | `docs/agent-notes/*vps*`, `docs/launch-readiness/` | `docs/deployments/`, `docs/architecture/` | active in private |
| BearHost doc (full) | *(replaced earlier)* | `archive/bearhost/bearhost-production.md` | stub remains public |

## Stays in public streamclone

- App code, migrations, local compose (`deploy/docker-compose.yml`, `release.yml`, `prod.yml`, `laptopworker-dev`)
- CI / GHCR release workflows
- `scripts/ops-stub.sh`, `make up`, `make compose-config-check`
- Stubs: `docs/bearhost-production.md`, `docs/hosted-production-ops.md`, `docs/production-artifact-contract.md`

## Desktop bundle

`scripts/package-release.sh` uses rsync allowlists + filename audit — no `*bearhost*`, `*streampulse-vps*`, `grafana/`, `prometheus/`, `charts/pulse/` in `dist/streamclone-*`.

## Merge gate

- [ ] Soak stable on `v0.3.0-rc7` (hub not critical/missing; containers healthy)
- [ ] Explicit operator approval
- [ ] **Not** before `v0.3.0-rc8` packaging decision
