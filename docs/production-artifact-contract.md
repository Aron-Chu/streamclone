# Production artifact contract

Public contract between **Streamclone** (app source + GHCR image builds) and **streampulse-ops** (private production execution). No host IPs or operator secrets belong in this document.

## Release identity

- Every production deploy is identified by an immutable **`IMAGE_TAG`** (git release tag matching `VERSION`, e.g. `v0.2.10`).
- Optional SHA tags may exist for traceability; production deploy uses the release tag unless break-glass.

## StreamPulse relationship

**StreamPulse production intentionally deploys Streamclone images.** StreamPulse is the hosted product surface (`streampulse.stream`, `api.streampulse.stream`, Chrome extension, portal), while Streamclone is the backend application and release train that builds the Go APIs, analytics BFF, workers, migrations, Redis/Postgres integrations, and supporting services.

The separated analytics boundary today is a **runtime/service boundary** inside this release train:

```text
streamclone repo -> ghcr.io/aron-chu/streamclone/analytics:${IMAGE_TAG}
				 -> analytics API container
				 -> analytics worker container(s)
				 -> migrate image with matching schema code
```

It is not yet a separately promoted `streampulse/analytics` image family or a second backend repository. If operators want StreamPulse-branded images later, use a deliberate promotion step from the same Streamclone release artifact rather than rebuilding from different source. The private ops repo may introduce aliases such as `STREAMPULSE_API_IMAGE_TAG`, but `analytics` and `migrate` must still be proven to come from one compatible source revision.

Do not treat the sibling `streamclone-pulse` repo as a backend image source. It owns the Chrome MV3 extension, the StreamPulse portal frontend, and product docs/specs; it calls the hosted API.

## GHCR images (built from `Aron-Chu/streamclone`)

Published on tag push via `.github/workflows/release-images.yml`:

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

## Invariant

For a given production deploy:

```
IMAGE_TAG(metadata) == IMAGE_TAG(analytics) == IMAGE_TAG(analytics-workers) == IMAGE_TAG(migrate)
```

Scraper may use a separate **`SCRAPER_IMAGE_TAG`** when built from `streamclone-scraper`.

Private ops must record any intentional per-service tag exception in deploy evidence. Mixed app tags without an exception are a release-discipline bug, not a reason to split repositories.

## Ops responsibilities (`streampulse-ops`, private)

- Pull all service images by pinned `IMAGE_TAG` — no app source bind mounts in default production path.
- Run `migrate` container from `ghcr.io/aron-chu/streamclone/migrate:${IMAGE_TAG}` before or during deploy.
- Store secrets on host (`/etc/streamclone/secrets/`) — never in git.
- Run smoke after deploy; record evidence in `docs/deployments/`.
- Keep `STREAMCLONE_VERSION` env aligned with `IMAGE_TAG` (host checkout `VERSION` is not deploy truth).
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

Redeploy the previous known-good **`IMAGE_TAG`**. Migrations may not be reversible across schema changes — review migration compatibility before rollback across major versions.

## Local / self-hosted

This public repo provides compose examples (`deploy/docker-compose.yml`, `release.yml`, `prod.yml`) for local and generic VM installs. Hosted production runbooks live in private **streampulse-ops**.
