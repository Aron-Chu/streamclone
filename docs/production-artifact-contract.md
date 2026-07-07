# Production source-build contract (Streamclone)

Public contract for **Streamclone CI image builds** and local/self-hosted releases. **StreamPulse hosted production promotion** (digest promotion, private compose, cutover) is defined separately in [`production-promotion-contract.md`](production-promotion-contract.md).

> **Migration in progress:** Production manifests are moving from `ghcr.io/aron-chu/streamclone/*` to digest-promoted `ghcr.io/aron-chu/streampulse/*`. See sibling [streamclone-image-exit-audit-2026-07.md](../../streamclone-pulse/docs/pulse-extension/evidence/streamclone-image-exit-audit-2026-07.md). Do not claim cutover complete until private **streampulse-ops** evidence shows promoted images and post-cutover smoke.

Private production execution (deploy, secrets, smoke, rollback) lives in **streampulse-ops**. No host IPs or operator secrets belong in this document.

## Release identity

- Every release is identified by an immutable **`IMAGE_TAG`** (git release tag matching `VERSION`, e.g. `v0.2.10`).
- Optional SHA tags may exist for traceability; production deploy uses the release tag unless break-glass.

## StreamPulse relationship

**Streamclone builds source images; StreamPulse ops promotes them for hosted production.** StreamPulse is the hosted product surface (`streampulse.stream`, `api.streampulse.stream`, Chrome extension, portal). Streamclone is the backend application and release train that builds Go APIs, analytics BFF, workers, migrations, Redis/Postgres integrations, and supporting services.

The separated analytics boundary is a **runtime/service boundary** inside this release train:

```text
streamclone repo -> ghcr.io/aron-chu/streamclone/analytics:${IMAGE_TAG}
                 -> analytics API container (hosted: promoted as streampulse/api)
                 -> analytics worker container(s) (hosted: promoted as streampulse/workers)
                 -> migrate image (hosted: promoted as streampulse/migrate)
```

Do not treat the sibling `streamclone-pulse` repo as a backend image source. It owns the Chrome MV3 extension, the StreamPulse portal frontend, and product docs/specs; it calls the hosted API.

## GHCR source images (built from `Aron-Chu/streamclone`)

Published on tag push via `.github/workflows/release-images.yml`. These are **source artifacts** for local dev, self-hosted Streamclone, and digest promotion to StreamPulse production:

| Image | Purpose |
|-------|---------|
| `ghcr.io/aron-chu/streamclone/metadata` | Metadata API |
| `ghcr.io/aron-chu/streamclone/chat` | Chat / IRC |
| `ghcr.io/aron-chu/streamclone/video` | Video / HLS relay |
| `ghcr.io/aron-chu/streamclone/analytics` | Analytics API + workers |
| `ghcr.io/aron-chu/streamclone/emote` | Emote pipeline |
| `ghcr.io/aron-chu/streamclone/frontend` | Web UI |
| `ghcr.io/aron-chu/streamclone/scraper` | Optional TwitchTracker scraper |
| `ghcr.io/aron-chu/streamclone/migrate` | DB migrations baked at tag |

## Source-build invariant

For a given release tag:

```
IMAGE_TAG(metadata) == IMAGE_TAG(analytics) == IMAGE_TAG(analytics-workers) == IMAGE_TAG(migrate)
```

Scraper may use a separate **`SCRAPER_IMAGE_TAG`** when built from `streamclone-scraper`.

Private ops must record any intentional per-service tag exception in deploy evidence. Mixed app tags without an exception are a release-discipline bug, not a reason to split repositories.

## Ops responsibilities (`streampulse-ops`, private)

Hosted promotion details: [`production-promotion-contract.md`](production-promotion-contract.md)

- Pull service images by pinned `IMAGE_TAG` (pre-cutover: `streamclone/*`; target: promoted `streampulse/*`) — no app source bind mounts in default production path.
- Run `migrate` from a digest compatible with the API/workers source revision.
- Store secrets on host (`/etc/streamclone/secrets/`) — never in git.
- Run smoke after deploy; record evidence in `docs/deployments/`.
- Keep deployed version env aligned with `IMAGE_TAG` (host checkout `VERSION` is not deploy truth).
- Set `PULSE_OPS_PROBE_TOKEN` for operator routes `/v1/internal/ops/*` (readiness + launch snapshot).
- Use public templates: [`docs/ops/promotion-manifest.template.md`](ops/promotion-manifest.template.md), [`docs/ops/cap250-soak-runbook.md`](ops/cap250-soak-runbook.md).

Operator probes (from laptop with SSH):

```bash
bash scripts/hosted-launch-probes.sh          # PULSE_PROBE_SSH_TARGET + token on host
bash scripts/ops/hosted-promotion-reconcile.sh # on VPS — tag/digest checklist
curl -H "X-Ops-Probe-Token: $TOKEN" http://127.0.0.1:8090/v1/internal/ops/launch-snapshot
```

Public `/v1/analytics/top100/readiness` stays blocked at the Caddy edge; launch probes use the internal ops path above.

## Rollback

Redeploy the previous known-good **`IMAGE_TAG`** (and digests from the private manifest). Migrations may not be reversible across schema changes — review migration compatibility before rollback across major versions.

## Local / self-hosted

This public repo provides compose examples (`deploy/docker-compose.yml`, `release.yml`, `prod.yml`) for local and generic VM installs. Hosted production runbooks live in private **streampulse-ops**.
