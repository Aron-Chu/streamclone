# Architecture

Streamclone is a self-hosted streaming and chat platform: Go microservices on the backend, React/Vite SPA on the frontend. The browser talks only to our services — never directly to Twitch, 7TV, or other upstream providers.

The stack is divided into three zones: Edge/Streaming, State/Ingestion, and Frontend.

```
Frontend SPA  ──REST──►  Metadata Service  :8081
              ──REST──►  Video Orchestrator :8082
              ──WS────►  Chat Gateway       :8083  (/v1/ws)
              ──HLS───►  MediaMTX           :8888
              ──img───►  MinIO / S3         :9000
```

## Go services

| Service | Host port | Responsibilities |
|---|---|---|
| Metadata | 8081 | Directory, categories, search, channel-id resolution; Redis cache with stale fallback; request coalescing via singleflight |
| Video Orchestrator | 8082 | PlaybackAccessToken + Usher handshake; spawns/reaps Stream Workers (streamlink piped to ffmpeg); listener keepalive; concurrency limit |
| Chat Gateway | 8083 | Anonymous IRC connections (`justinfan` convention); IRCv3 tag parsing; Trie-based emote tokenization; micro-batch fan-out over one WebSocket per client session (`/v1/ws`) |
| Emote Service | 8084 | Emote CRUD, emote sets, channel-set mapping; async asset pipeline (libvips CLI → WebP 1×–4×); 7TV v3 seeder; Redis emote dictionary + live delta publication |
| Analytics Service | 8086 | Viewed-channel stream analytics; Helix viewer samples; IRC chat/emote counting; compact JSONB minute rollups; 30-day retention |

The Video Orchestrator shells out to **streamlink** (upstream HLS puller) piped to **ffmpeg** (`-c copy`), publishing RTMP to MediaMTX. The Emote Service uses the **libvips CLI** for WebP scale generation at four sizes (1×/2×/3×/4×).

## Infrastructure

| Component | Host ports | Role |
|---|---|---|
| Redis 7 | 6379 | Metadata cache, emote dictionary hashes, pub/sub fan-out |
| PostgreSQL 16 | 5432 | Durable emote schema (emotes, sets, items, channels, jobs) |
| MinIO | 9000 (API), 9001 (console) | Local S3-compatible object store for emote WebP assets |
| MediaMTX | 1935 (RTMP ingest), 8888 (HLS output) | Media edge; bounded HLS ring buffer (5 segments) |

All services are wired together in [`deploy/docker-compose.yml`](../deploy/docker-compose.yml).

Local development uses the Caddy reverse proxy at **`http://localhost:8090`** as the single browser entrypoint. HLS (`/live/*`) is proxied to MediaMTX with a CDN auth header so MediaMTX 1.18+ session cookies work on plain HTTP localhost (see `.kiro/steering/playback.md`). Clipper API traffic uses `/v1/clipper/*`; Clip Studio is `/studio/:jobId` in the SPA.
