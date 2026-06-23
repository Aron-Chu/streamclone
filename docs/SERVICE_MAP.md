# Service map

All browser and agent probes should use **`http://localhost:8090`** (Caddy `local-proxy`). Host-mapped ports below are for debugging only.

See also: [ENVIRONMENT.md](ENVIRONMENT.md), [MCP.md](MCP.md), `deploy/Caddyfile.local-tunnel`.

---

## Edge proxy

| Service | Compose | Host port | Internal | Public route (`:8090`) | Entry | Health |
|---------|---------|-----------|----------|------------------------|-------|--------|
| **local-proxy** (Caddy) | `local-proxy` | **8090→80** | 80 | All routes | `deploy/Caddyfile.local-tunnel` | `GET /` (frontend) |
| **frontend** (nginx + SPA) | `frontend` | *(via proxy)* | 80 | `/` (default) | `frontend/` | `GET /` inside container |

---

## Core Go APIs

Go services listen on **`:8080` inside the container**. Health default: `GET /healthz` via `cmd/healthcheck` (metadata uses a deeper probe).

| Service | Compose | Host port | Caddy route | Main entry | Package / folder | Dependencies |
|---------|---------|-----------|-------------|------------|------------------|--------------|
| **metadata** | `metadata` | 8081 | `/v1/*` (catch-all after specific routes) | `cmd/metadata/main.go` | `internal/metadata/` | Postgres, Redis |
| **video** | `video` | 8082 | `/v1/stream`, `/v1/stream/*` | `cmd/video/main.go` | `internal/video/` | Redis, **MediaMTX** |
| **chat** | `chat` | 8083 | `/v1/auth*`, `/v1/me`, `/v1/ws`, … | `cmd/chat/main.go` | `internal/chat/` | Redis |
| **emote** | `emote` | 8084 | `/v1/emotes*`, `/v1/channels/*/emotes*` | `cmd/emote/main.go` | `internal/emote/` | Postgres, Redis, **MinIO** |
| **analytics** | `analytics` | 8086 | `/v1/analytics`, `/v1/analytics/*`, `/v1/extension`, `/v1/extension/*`, `/v1/pulse`, `/v1/pulse/*` | `cmd/analytics/main.go` | `internal/analytics/` (+ extension BFF in `extension_api.go`) | Postgres, Redis, emote (URL), **scraper** (optional) |
| **storygraph** (Pulse Wire API) | `storygraph` | 8087 | `/v1/pulse-wire/*`, `/v1/channels/*/spread` | `cmd/storygraph/main.go` | `internal/storygraph/`, `internal/social/` | Postgres, Redis, metadata, analytics; optional scraper, media-matcher, x-ingest |

**Emote assets:** `/emotes/*` → MinIO `:9000`.

**HLS playback:** `/live/{channel}/*.m3u8` → MediaMTX `:8888` (Bearer `streamclone-local-hls-cdn` set by Caddy).

**Setup control:** `/v1/setup-control/*` → host `host.docker.internal:9191` (desktop launcher).

**Clipper / ReplayForge:** `/v1/clipper/*` → host `:8095` (ReplayForge API, outside compose).

---

## Data and media infrastructure

| Service | Compose | Host port | Used by | Health |
|---------|---------|-----------|---------|--------|
| **postgres** | `postgres` | 5432 | All Go services | `pg_isready` |
| **redis** | `redis` | 6379 | chat, metadata, video, emote, analytics, storygraph | `redis-cli ping` |
| **minio** | `minio` | 9000 (API), 9001 (console) | emote asset storage | — |
| **mediamtx** | `mediamtx` | 1935 (RTMP), 8888 (HLS) | video relay | — |
| **migrate** | `migrate` | — | one-shot schema | exit 0 |

---

## Optional profiles

### `scraper`

| Service | Compose | Host port | Route | Entry | Notes |
|---------|---------|-----------|-------|-------|-------|
| **scraper** | `scraper` (profile) | 8000 | Internal only (`SCRAPER_API_URL`) | Sibling `streamclone-scraper` or GHCR image | `POST /v2/scrape`; analytics + storygraph client |

Steering: `docs/scraper-cloudflare-and-proxy.md`.

### Azure hybrid archive plane

| Deployment | Hostname (Tailscale) | Scraper reachability | Notes |
|------------|----------------------|----------------------|-------|
| **Mode A — remote scraper** | `azure-streamclone` | `http://azure-streamclone:8000/v2/scrape` | Local sets `SCRAPER_API_URL`; local scraper profile off |
| **Mode B — archive plane** | `azure-streamclone` | internal `http://scraper:8000/v2/scrape` on VM | Azure Postgres + workers; local workers off via `profile-local-hybrid.env` |

Validation: `make hybrid-preflight`, `scripts/azure-hybrid-smoke.ps1`. Full runbook: [azure-archive-plane.md](azure-archive-plane.md).

### `pulse`

| Service | Compose | Host port | Notes |
|---------|---------|-----------|-------|
| **influxdb** | `influxdb` | 18086→8086 | Analytics time-series writes |
| **prometheus** | `prometheus` | 9090 | Metrics |
| **grafana** | `grafana` | 3000 | Dashboards (`deploy/grafana/`) |

### `pulse-wire`

| Service | Compose | Host port | Notes |
|---------|---------|-----------|-------|
| **storygraph** | core + pulse-wire env | 8087 | Ingest schedulers when enabled |
| **media-matcher** | `media-matcher` | 8001 | `/healthz` |
| **x-ingest** | `x-ingest` | 8098 | X/emusks bridge; needs `X_AUTH_TOKEN` |

### `pulse-wire-semantic`

| Service | Compose | Notes |
|---------|---------|-------|
| **migrate-semantic** | one-shot | Optional PG extensions under `migrations/optional/` |

---

## Host-only (not in compose)

| Service | Port | Purpose |
|---------|------|---------|
| **setup-control** | 9191 | Desktop install / env reload API |
| **ReplayForge clipper** | 8095 API, 8096 UI | Clip Studio (`../replayforge`) |

---

## Quick diagnostic URLs (via `:8090`)

| Check | URL |
|-------|-----|
| Auth debug | `GET /v1/auth/debug` |
| Stream diagnostics | `GET /v1/stream/diagnostics?channel={login}` |
| HLS manifest | `GET /live/{login}/index.m3u8` |
| Analytics health | `GET /v1/analytics/...` (service `/healthz` on :8086 direct) |
| Extension BFF health | `GET /v1/extension/health` |
| Pulse Wire feed | `GET /v1/pulse-wire/...` |

MCP equivalents: `stack_health`, `playback_probe`, `twitch_auth_status`, `scraper_probe`.
