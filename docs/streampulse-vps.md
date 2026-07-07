# hosted-production-vps (hosted production)

Public, no-secrets reference for agents and contributors. **Do not put operator secrets, host passwords, or production env files in this repo.**

## Status

**hosted-production-vps** is the current hosted production source of truth (SoT):

| Role | Host |
|------|------|
| Public API (via Cloudflare) | `https://api.streampulse.stream` |
| Production VPS | **hosted-production-vps** (`hosted-production-vps`) |
| Hot data plane | Postgres, Redis, Caddy `:8090`, analytics + workers on the VPS |
| Deploy / smoke / rollback evidence | Private **streampulse-ops** (not in this public repo) |

Production cutover: **2026-07-02**. See [requirements/corpus-scaling-observability.md](requirements/corpus-scaling-observability.md).

## Public repo vs operator lane

| Layer | Where |
|-------|--------|
| Application source, migrations, CI, GHCR images | This public **streamclone** repo |
| Pinned `IMAGE_TAG` deploy, secrets, production compose overlays | Private **streampulse-ops** |
| Authoritative production env | `streampulse-ops/env/production.local.env` (never commit) |

Public `deploy/env/profile-bearhost-pulse.env` and `deploy/docker-compose.bearhost*` paths are **legacy / rollback references only** — not live production config.

## BearHost (rollback / archive only)

**BearHost** (`legacy-rollback-host`) was pre-cutover production. It is **rollback-only** until explicitly retired.

- Public stub: [bearhost-production.md](bearhost-production.md)
- Operator archive: `streampulse-ops/archive/bearhost/`

Agents must **not** SSH to BearHost or treat BearHost scripts/env as current production unless doing a documented rollback.

## Agent probes (read-only)

Safe without mutating production:

```bash
curl -s https://api.streampulse.stream/v1/extension/health
curl -s https://api.streampulse.stream/v1/public/hub | head -c 500
```

From this repo (hosted boundary checks):

```bash
bash deploy/smoke/test-013b-hosted.sh
bash scripts/pulse-hosted-boundary-smoke.sh
```

Optional: **streamclone-hosted-data** MCP (read-only Postgres/Redis via SSH tunnel) — see [MCP.md](MCP.md) and `deploy/env/profile-hosted-data-mcp.env.example`.

**Local stack** (`http://localhost:8090`) is for Streamclone backend development only — not representative hosted corpus scale. **StreamPulse portal dev** (`streampulse-web`) reads `https://api.streampulse.stream` by default; see sibling [streamclone-pulse local-dev-runbook](https://github.com/Aron-Chu/streamclone-pulse/blob/master/docs/website-portal/local-dev-runbook.md).

## Launch hardening (2026-07)

**Pre-cutover:** hosted production manifests may still reference **Streamclone GHCR source tags** (`ghcr.io/aron-chu/streamclone/*`). **Target:** digest-promoted **StreamPulse images** (`ghcr.io/aron-chu/streampulse/*`) per promotion contract. Promotion discipline, caps, Redis/container limits, and soak evidence live in private **streampulse-ops**.

| Doc | Location |
|-----|----------|
| Image namespace exit audit | Sibling [`streamclone-image-exit-audit-2026-07.md`](../../streamclone-pulse/docs/pulse-extension/evidence/streamclone-image-exit-audit-2026-07.md) |
| Production promotion contract | [docs/production-promotion-contract.md](production-promotion-contract.md) |
| Source-build contract | [docs/production-artifact-contract.md](production-artifact-contract.md) |
| Production artifact decision (launch) | Sibling [`production-artifact-decision-2026-07.md`](../../streamclone-pulse/docs/pulse-extension/evidence/production-artifact-decision-2026-07.md) |
| Full review + 7-day soak plan | Sibling [`streamclone-pulse/docs/pulse-extension/evidence/improvements.md`](../../streamclone-pulse/docs/pulse-extension/evidence/improvements.md) |
| Promotion manifest template | [docs/ops/promotion-manifest.template.md](ops/promotion-manifest.template.md) |
| Cap-250 soak runbook | [docs/ops/cap250-soak-runbook.md](ops/cap250-soak-runbook.md) |
| Staged resource limits | [docs/ops/hosted-limits-staged-runbook.md](ops/hosted-limits-staged-runbook.md) |

Operator probes (after SSH + ops token on host):

```bash
bash scripts/hosted-launch-probes.sh
bash scripts/load/hosted-cap250-soak-monitor.sh
curl -fsS -H "X-Ops-Probe-Token: $PULSE_OPS_PROBE_TOKEN" http://127.0.0.1:8090/v1/internal/ops/readiness?topN=500
```

## Related docs

- [hosted-production-ops.md](hosted-production-ops.md) — operator entrypoints
- [production-artifact-contract.md](production-artifact-contract.md) — source-build contract
- [production-promotion-contract.md](production-promotion-contract.md) — hosted promotion contract
- [ops-migration-manifest.md](ops-migration-manifest.md) — public vs private file boundary
- [requirements/corpus-scaling-observability.md](requirements/corpus-scaling-observability.md) — live / VOD / corpus planes on hosted-production-vps
