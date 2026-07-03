# Production artifact contract

Public contract between **Streamclone** (app source + GHCR image builds) and **streampulse-ops** (private production execution). No host IPs or operator secrets belong in this document.

## Release identity

- Every production deploy is identified by an immutable **`IMAGE_TAG`** (git release tag matching `VERSION`, e.g. `v0.2.10`).
- Optional SHA tags may exist for traceability; production deploy uses the release tag unless break-glass.

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

## Ops responsibilities (`streampulse-ops`, private)

- Pull all service images by pinned `IMAGE_TAG` — no app source bind mounts in default production path.
- Run `migrate` container from `ghcr.io/aron-chu/streamclone/migrate:${IMAGE_TAG}` before or during deploy.
- Store secrets on host (`/etc/streamclone/secrets/`) — never in git.
- Run smoke after deploy; record evidence in `docs/deployments/`.

## Rollback

Redeploy the previous known-good **`IMAGE_TAG`**. Migrations may not be reversible across schema changes — review migration compatibility before rollback across major versions.

## Local / self-hosted

This public repo provides compose examples (`deploy/docker-compose.yml`, `release.yml`, `prod.yml`) for local and generic VM installs. Hosted production runbooks live in private **streampulse-ops**.
