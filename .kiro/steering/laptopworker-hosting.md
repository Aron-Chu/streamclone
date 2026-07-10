# Hosting topology — legacy-rollback-host, laptopworker, Windows PC

Where Streamclone **core watch stack** workloads run. `:8090` is the Streamclone watch API only; StreamPulse extension and portal use a separate hosted backend.

## Three hosts

| Host | Role | URL / access |
|------|------|----------------|
| **legacy-rollback-host** | Production ingress, scrape, corpus, backfill | `https://api.streampulse.stream` (StreamPulse hosted API) |
| **laptopworker** | Private tailnet dev hub — core watch stack only | `http://laptopworker:8090` (Tailscale MagicDNS) |
| **Windows PC** | Cursor, local dev stack, remote control | `http://localhost:8090` or `http://127.0.0.1:8090` |

## legacy-rollback-host (always-on public)

**Run here:**

- TwitchTracker / Camoufox **scraper** profile
- Corpus workers (silver/gold, backfill) — StreamPulse backend private repo
- Public HTTPS ingress (Caddy / API)
- Grafana / observability (hosted profiles)
- Network-heavy sync against TwitchTracker and corpus

**Do not expect on laptop:** scraper, workers, storygraph ingest, bronze/tier0 backfill.

Ref: `docs/bearhost-production.md`, `make bearhost-*`, `make local-vps-only` on PC.

## laptopworker (private tailnet dev hub)

**Run here:**

- Core watch compose: Caddy `:8090`, frontend, metadata, video, chat, emote, postgres, redis, minio, mediamtx
- Local dev DB (empty/small — not production corpus)

**Do not run here:**

- StreamPulse extension BFF routes (`/v1/extension/*`) — those route to `https://api.streampulse.stream` or the local **streampulse-backend** BFF (`:8081`)
- Local scraper profile (`STREAMCLONE_DISABLE_LOCAL_SCRAPER=true`)
- `analytics-workers`, storygraph, Pulse Wire ingest
- Public port forwards from home router
- Exposing `:5432`, `:6379`, `:9000` on LAN/WAN

**Security:** `:8090` tailnet-only via root-owned `/usr/local/sbin/streamclone-laptopworker-firewall` (INPUT early drop + DOCKER-USER). SSH on `tailscale0` only by default.

**Control from Windows:** `scripts\laptopworker-remote.cmd` — see `docs/laptopworker-dev.md`.

## Windows PC (primary dev machine)

**Run here:**

- Cursor / VS Code (multi-root: streamclone + streamclone-pulse)
- Optional **local** full watch stack (`make up` — watch/playback/chat/emotes only)
- `scripts\laptopworker-remote.cmd` to operate laptop without walking over

**Typical split:** daily UI/watch work → localhost or laptopworker; scrape/corpus tests → hosted API; StreamPulse extension/portal work → see StreamPulse repo targets below.

## Backend targets

| Surface | Default backend | Local override |
|---------|----------------|----------------|
| **Streamclone watch UI** | `http://localhost:8090` or `http://laptopworker:8090` | — |
| **StreamPulse extension** | `https://api.streampulse.stream` | **streampulse-backend** BFF `:8081` (`npm run dev:local` in streamclone-pulse) |
| **StreamPulse portal** | `https://api.streampulse.stream` | **streampulse-backend** BFF `:8081` |

Health checks:

```bash
# Streamclone core watch
curl http://localhost:8090/v1/metadata/health
# StreamPulse hosted
curl -fsS https://api.streampulse.stream/v1/extension/health
```

**Do not** use `localhost:8090` as the backend for StreamPulse extension or portal development — the core stack does not have `/v1/extension/*`, `/v1/public/hub`, or Pulse analytics routes.

## StreamPulse work → other repos

| Need | Repo |
|------|------|
| Extension overlay + portal UI | **streamclone-pulse** (`src/`, `streampulse-web/`) |
| BFF API, ingest, packages | private **streampulse-backend** |
| Deploy, secrets, SSH, soak | private **streampulse-ops** |

## Agent guardrails

1. Never enable scraper/corpus/storygraph on laptop profile or compose overlay.
2. Never commit secrets; OAuth stays in `deploy/env/oauth-bundle.env` (gitignored).
3. Laptop firewall changes → edit `scripts/laptopworker/sbin/streamclone-laptopworker-firewall`, then `scripts\laptopworker-remote.cmd setup` (refreshes `/usr/local/sbin`).
4. BearHost owns scrape; laptop is UI/playback/chat/emotes dev only.
5. Do not bind laptop Caddy to `0.0.0.0` without firewall helper active.
6. Do not route StreamPulse extension or portal API calls to `:8090` — those belong on the hosted API or local **streampulse-backend** BFF.

## Frontend / website placement

| Surface | What it is | Where it lives |
|---------|------------|----------------|
| **Streamclone UI** (directory, channel, VOD shell) | Docker `frontend` + Caddy | `:8090` on laptop / PC / hosted stack — **not** a separate static host |
| **StreamPulse public site** (`streampulse.stream`) | `streamclone-pulse/streampulse-web` | **Cloudflare Pages** (static `dist/`); API = `https://api.streampulse.stream` |
| **Pulse extension** | MV3 in `streamclone-pulse` | User's browser; default backend = `https://api.streampulse.stream` |

Do not put the StreamPulse marketing/portal site on laptopworker — it is a separate repo deployed to Cloudflare Pages.

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

Enable with `make up` + **`pulse` profile** on PC or BearHost when you want Grafana dashboards — **not required on laptopworker** (core dev hub only). Postgres + analytics service remain authoritative; Influx is a mirror for charts.

## Related docs

- `docs/laptopworker-dev.md` — runbook
- `docs/workspace.md` — two-repo layout
- `docs/bearhost-production.md` — VPS
- `docs/agent-context.md` — runtime probes
- `docs/streampulse-product-boundary.md` — what lives where
