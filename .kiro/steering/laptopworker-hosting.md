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

## Related docs

- `docs/laptopworker-dev.md` — runbook
- `docs/workspace.md` — two-repo layout
- `docs/bearhost-production.md` — VPS
- `docs/agent-context.md` — runtime probes
