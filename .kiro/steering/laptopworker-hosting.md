# Hosting topology — legacy-rollback-host, laptopworker, Windows PC

Where Streamclone workloads run. Agents should not move network-heavy jobs to laptopworker or expose laptop Postgres/Redis publicly.

## Three hosts

| Host | Role | URL / access |
|------|------|----------------|
| **legacy-rollback-host** | Production ingress, scrape, corpus, backfill, hosted API | `https://api.streampulse.stream` |
| **laptopworker** | Private tailnet dev hub — core stack only | `http://laptopworker:8090` (Tailscale MagicDNS) |
| **Windows PC** | Cursor, local dev stack, remote control | `http://localhost:8090` or `http://127.0.0.1:8090` |

## legacy-rollback-host (always-on public)

**Run here:**

- TwitchTracker / Camoufox **scraper** profile
- `analytics-workers` (corpus, silver/gold, backfill)
- Pulse Wire **storygraph** ingest (when enabled)
- Public HTTPS ingress (Caddy / API)
- Grafana / observability (hosted profiles)
- Network-heavy sync against TwitchTracker and corpus

**Do not expect on laptop:** scraper, workers, storygraph ingest, bronze/tier0 backfill.

Ref: `docs/bearhost-production.md`, `make bearhost-*`, `make local-vps-only` on PC.

## laptopworker (private tailnet dev hub)

**Run here:**

- Core compose: Caddy `:8090`, frontend, metadata, video, chat, emote, analytics **API**, postgres, redis, minio, mediamtx
- Extension BFF routes (`/v1/extension/*`) for tailnet dev
- Local dev DB (empty/small — not production corpus)

**Do not run here:**

- Local scraper profile (`STREAMCLONE_DISABLE_LOCAL_SCRAPER=true`)
- `analytics-workers`, storygraph, Pulse Wire ingest
- Public port forwards from home router
- Exposing `:5432`, `:6379`, `:9000` on LAN/WAN

**Security:** `:8090` tailnet-only via root-owned `/usr/local/sbin/streamclone-laptopworker-firewall` (INPUT early drop + DOCKER-USER). SSH on `tailscale0` only by default.

**Control from Windows:** `scripts\laptopworker-remote.cmd` — see `docs/laptopworker-dev.md`.

## Windows PC (primary dev machine)

**Run here:**

- Cursor / VS Code (multi-root: streamclone + streamclone-pulse)
- Optional **local** full stack (`make up`, `make up-scraper` for experiments)
- StreamPulse extension dev (default backend `http://localhost:8090`)
- `scripts\laptopworker-remote.cmd` to operate laptop without walking over

**Typical split:** daily UI/extension work → localhost **or** laptopworker; scrape/corpus tests → BearHost API or VPS stack, not laptop.

## Backend targets (extension / portal)

| Mode | Backend |
|------|---------|
| Windows local stack | `http://localhost:8090` |
| Laptop dev hub | `http://laptopworker:8090` |
| Production / BearHost | `https://api.streampulse.stream` |

Health: `GET /v1/extension/health` on the chosen base URL.

## Agent guardrails

1. Never enable scraper/corpus/storygraph on laptop profile or compose overlay.
2. Never commit secrets; OAuth stays in `deploy/env/oauth-bundle.env` (gitignored).
3. Laptop firewall changes → edit `scripts/laptopworker/sbin/streamclone-laptopworker-firewall`, then `scripts\laptopworker-remote.cmd setup` (refreshes `/usr/local/sbin`).
4. BearHost owns scrape; laptop is UI/playback/chat/emotes/BFF dev only.
5. Do not bind laptop Caddy to `0.0.0.0` without firewall helper active.

## Frontend / website placement

| Surface | What it is | Where it lives |
|---------|------------|----------------|
| **Streamclone UI** (directory, channel, VOD shell) | Docker `frontend` + Caddy | `:8090` on laptop / PC / BearHost stack — **not** a separate static host |
| **StreamPulse public site** (`streampulse.stream`) | `streamclone-pulse/streampulse-web` | **Cloudflare Pages** (static `dist/`); API = `https://api.streampulse.stream` |
| **Pulse extension** | MV3 in `streamclone-pulse` | User's browser; backend URL in extension Options |

Do not put the StreamPulse marketing/portal site on laptopworker — it is a separate repo deploy to Cloudflare.

## When to add Kubernetes / ArgoCD

**Not needed today.** Streamclone uses **Docker Compose** on BearHost and laptopworker; that matches team size and ops burden.

Consider K8s + ArgoCD only if **all** of these become true:

- Multiple long-lived environments (staging + prod + region) with the same manifests
- Several stateful services you want unified rollouts/rollback for
- A dedicated ops owner for cluster lifecycle

Until then: Compose + git pull + `laptopworker-remote.cmd update` / BearHost deploy scripts + Cloudflare Pages for the portal is the intended model. See `docs/azure-archive-cicd.md` (ArgoCD noted for K8s app GitOps only, not archive plane).

## When to use InfluxDB

**Optional dev/ops layer**, not product source of truth.

| Use InfluxDB | Do not use InfluxDB for |
|--------------|-------------------------|
| Emote Pulse Grafana charts (`pulse` compose profile) | Public API, extension data, portal analytics |
| Local time-series mirror of minute rollups | Corpus, scraper, backfill state |

Enable with `make up` + **`pulse` profile** on PC or BearHost when you want Grafana dashboards — **not required on laptopworker** (core dev hub only). Postgres + analytics service remain authoritative; Influx is a mirror for charts (`docs/scraping-archive/requirements.md`, `docs/options.md`).

BearHost observability may use **Prometheus** (VPS tunnel `:3001`) separately from local Influx Pulse on `:3000` — do not mix them up (`docs/site-links.md`).

## Related docs

- `docs/laptopworker-dev.md` — runbook
- `docs/workspace.md` — two-repo layout
- `docs/bearhost-production.md` — VPS
- `docs/agent-context.md` — runtime probes
