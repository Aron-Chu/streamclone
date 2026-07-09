# Service map

All browser and agent probes should use **`http://localhost:8090`** (Caddy `local-proxy`). Host-mapped ports below are for debugging only.

See also: [ENVIRONMENT.md](ENVIRONMENT.md), [MCP.md](MCP.md), `deploy/Caddyfile.local-tunnel`.

**Scope:** core Twitch-replica stack only. StreamPulse BFF routes are owned by private **streampulse-backend** — not part of this product surface. See [streampulse-product-boundary.md](streampulse-product-boundary.md).

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

**Emote assets:** `/emotes/*` → MinIO `:9000`.

**HLS playback:** `/live/{channel}/*.m3u8` → MediaMTX `:8888` (Bearer `streamclone-local-hls-cdn` set by Caddy).

**Setup control:** `/v1/setup-control/*` → host `host.docker.internal:9191` (desktop launcher).

**Clipper / ReplayForge:** `/v1/clipper/*` → host `:8095` (ReplayForge API, outside compose).

---

## Data and media infrastructure

| Service | Compose | Host port | Used by | Health |
|---------|---------|-----------|---------|--------|
| **postgres** | `postgres` | 5432 | Core Go services | `pg_isready` |
| **redis** | `redis` | 6379 | chat, metadata, video, emote | `redis-cli ping` |
| **minio** | `minio` | 9000 (API), 9001 (console) | emote asset storage | — |
| **mediamtx** | `mediamtx` | 1935 (RTMP), 8888 (HLS) | video relay | — |
| **migrate** | `migrate` | — | one-shot schema | exit 0 |

---

## Host-only (not in compose)

| Service | Port | Purpose |
|---------|------|---------|
| **setup-control** | 9191 | Desktop install / env reload API |
| **ReplayForge** | 8095 | Optional Clip Studio (sibling checkout) |

---

## Legacy / split in progress

The compose tree may still include legacy **analytics** and **scraper** services during migration. They are **not** part of the Streamclone desktop product. Do not document or route agent work through them from this repo — see [docs/split/route-api-matrix.md](split/route-api-matrix.md).

---

## Smoke probes (core)

```sh
make up
make smoke
curl -fsS http://localhost:8090/v1/metadata/health
curl -w "%{http_code}" http://localhost:8090/live/{channel}/main_stream.m3u8
```

MCP: `stack_health`, `playback_probe`, `twitch_auth_status`.
