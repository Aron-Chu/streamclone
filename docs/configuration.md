# Configuration

All settings are read from environment variables. Copy [`.env.example`](../.env.example) to `.env` and adjust as needed. No secrets are hardcoded in source.

The nginx frontend image writes `/config.js` at container startup from `VITE_*` runtime env vars so API origins can change without rebuilding the Vite bundle.

## Core infrastructure

| Variable | Description |
|---|---|
| `DATABASE_URL` | PostgreSQL DSN, e.g. `postgres://app:app@postgres:5432/streamclone?sslmode=disable` |
| `REDIS_URL` | Redis connection URL, e.g. `redis://redis:6379/0` |
| `CURATOR_API_TOKEN` | Bearer token protecting curator/admin API endpoints — **set a strong random value before any non-local deployment** |
| `BACKEND_VERSION` | Version label surfaced in stream diagnostics |

## Twitch upstream

| Variable | Description |
|---|---|
| `TWITCH_GQL_URL` | Upstream internal GraphQL endpoint |
| `TWITCH_CLIENT_ID` | Public static Twitch web client identifier sent with GQL requests |
| `TWITCH_USHER_URL` | Usher base URL for master HLS playlist retrieval |
| `TWITCH_IRC_URL` | Upstream IRC-over-WebSocket endpoint |
| `TWITCHTRACKER_API_URL` | TwitchTracker basic API base used for cached channel stat summaries |
| `META_CACHE_TTL` | TTL for fresh metadata cache entries (default `30s`) |
| `STALE_TTL` | How long a stale metadata fallback is retained (default `24h`) |

## Reddit / LSF

| Variable | Description |
|---|---|
| `REDDIT_API_URL` | Reddit API base used for cached LivestreamFail listing/search enrichments |
| `REDDIT_PROVIDER` | LSF provider mode: `auto`, `official`, `public_json`, `third_party`, `firecrawl`, or `off` |
| `REDDIT_CLIENT_ID` / `REDDIT_CLIENT_SECRET` | Optional Reddit OAuth credentials for official Data API reads |
| `REDDIT_ACCESS_TOKEN` | Optional pre-provisioned Reddit bearer token |
| `REDDIT_HTML_FALLBACK` | Enables bounded local HTML listing fallback after official/public JSON fail |
| `REDDIT_THIRD_PARTY_URL` / `REDDIT_THIRD_PARTY_KEY` | Optional third-party LSF/search adapter |
| `FIRECRAWL_API_URL` / `FIRECRAWL_API_KEY` | Scraper API for TwitchTracker HTML (analytics historical sync, stream history) and optional Reddit LSF |

**Firecrawl example** (skips Reddit official/public JSON):

```sh
REDDIT_PROVIDER=firecrawl
FIRECRAWL_API_URL=https://api.firecrawl.dev/v2/scrape
FIRECRAWL_API_KEY=...
```

**Reddit official example** (create a classic app at https://www.reddit.com/prefs/apps):

```sh
REDDIT_PROVIDER=official
REDDIT_CLIENT_ID=...
REDDIT_CLIENT_SECRET=...
REDDIT_ACCESS_TOKEN=
REDDIT_HTML_FALLBACK=true
```

If `REDDIT_ACCESS_TOKEN` is blank, the metadata service uses `client_credentials`. Blocked providers back off for ~45 seconds before retry.

## Streaming

| Variable | Description |
|---|---|
| `STREAM_IDLE_TIMEOUT` | How long a channel with zero listeners is kept alive (default `60s`) |
| `MAX_CONCURRENT_STREAMS` | Maximum simultaneous Stream Workers (default `20`) |
| `STREAM_WORKER_BACKENDS` | Ordered backends, usually `streamlink,direct_hls` |
| `DEFAULT_STREAM_QUALITY` | Default quality sent to the Video Orchestrator (`best`) |
| `MEDIAMTX_RTMP` | MediaMTX RTMP ingest address for workers |
| `HLS_INTERNAL_BASE` | Internal MediaMTX HLS base for readiness probe |
| `HLS_PUBLIC_BASE` | Public HLS base returned to the browser; default local flow: `http://localhost:8090` |

MediaMTX HLS session auth is configured in `deploy/mediamtx.yml` (`hlsCDNSecret`) and must match the `Authorization: Bearer` header Caddy sends on `/live/*` routes. See `.kiro/steering/playback.md`.

## Chat

| Variable | Description |
|---|---|
| `BATCH_WINDOW_MS` | Chat burst coalescing window after first flush (default `20`) |
| `CLIENT_SEND_QUEUE` | Per-client outbound WebSocket queue depth (default `256`) |
| `MAX_CHANNELS_PER_SOCKET` | Maximum IRC JOINs per upstream anonymous WebSocket (default `30`) |
| `MAX_RETAINED_MESSAGES` | Frontend chat buffer cap (default `200`) |

## Auth

| Variable | Description |
|---|---|
| `TWITCH_OAUTH_CLIENT_ID` | Twitch application client ID for viewer login and chat posting |
| `TWITCH_OAUTH_CLIENT_SECRET` | Twitch application client secret for Helix and device-code login |
| `FRONTEND_ORIGIN` | Browser origin for credentialed chat auth APIs (default `http://localhost:8090`) |
| `AUTH_COOKIE_SECRET` | HMAC secret for signed chat-session cookies |
| `AUTH_COOKIE_SAMESITE` | Cookie SameSite mode; use `none` for HTTPS tunnel on a different domain |
| `TWITCH_AUTH_SCOPES` | OAuth scopes (default `chat:read chat:edit user:read:follows`) |
| `TWITCH_DEV_TOKEN_IMPORT_ENABLED` | Enables localhost-only token import route and UI |
| `APP_DOMAIN` | Public HTTPS host for production Caddy proxy |
| `ACME_EMAIL` | Optional email for Let's Encrypt certificate notices |
| `PUBLIC_ORIGIN` | Browser-visible origin; default local: `http://localhost:8090` |
| `PUBLIC_ORIGIN_WS` | Paired WS origin; default local: `ws://localhost:8090` |

Local Twitch login helpers: `make twitch TWITCH_ACTION=configure|sync|token|local-auth`, or `powershell -ExecutionPolicy Bypass -File scripts/twitch-auth.ps1 -Action local-auth` on Windows. See [oauth.md](../oauth.md).

## Emotes and object storage

| Variable | Description |
|---|---|
| `SEVENTV_API_URL` | 7TV v3 API base URL for seeding |
| `SEVENTV_CDN_URL` | 7TV CDN base URL for seed emote assets |
| `FFZ_API_URL` | FrankerFaceZ v1 API base URL |
| `S3_ENDPOINT` | Object store endpoint (`http://minio:9000` for MinIO, or AWS/R2 URL) |
| `S3_BUCKET` | Bucket name for emote WebP assets |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | Object store credentials |
| `CDN_PUBLIC_BASE` | Public base URL prepended to emote asset paths |
| `EMOTE_IMPORT_CONCURRENCY` | Parallel provider asset downloads per channel ensure (default `8`) |
| `EMOTE_WORKER_CONCURRENCY` | Background emote asset workers (default `8`) |
| `EMOTE_DICTIONARY_DEBOUNCE_MS` | Batches Redis dictionary rebuilds (default `3000`) |
| `DELTA_DEBOUNCE_MS` | Debounce for emote dictionary live-delta rebuilds (default `300`) |

Switching between MinIO and AWS S3 or Cloudflare R2 requires only changing `S3_ENDPOINT` and credentials — no code changes.

## Analytics

| Variable | Description |
|---|---|
| `MAX_CONCURRENT_TRACKED_CHANNELS` | Maximum channels tracked until offline (default `50`) |
| `ANALYTICS_POLL_INTERVAL` | Helix stream sampling interval (default `15s`) |
| `ANALYTICS_RETENTION_DAYS` | Retention for analytics streams and rollups (default `30`) |
| `ANALYTICS_TOP_EMOTES_PER_MINUTE` | Max emote keys per minute JSONB rollup (default `200`) |

For the full list and defaults, see [`.env.example`](../.env.example).
