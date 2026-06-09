# Playback / HLS Steering

## Purpose

Live channel playback relays Twitch through the Video Orchestrator into MediaMTX HLS. The browser loads manifests and segments through the Caddy proxy at **`http://localhost:8090/live/{channel}/...`**.

## Stack

- Orchestrator: `cmd/video`, `internal/video/orchestrator`
- Workers: `streamlink` (default) or `direct_hls` (FFmpeg from usher URL via `/v1/stream/proxy`)
- Media edge: `deploy/mediamtx.yml` — RTMP ingest `:1935`, HLS output `:8888`
- Proxy routes: `deploy/Caddyfile.local-tunnel` — `@hls` / `@hls_local` → `mediamtx:8888`
- Frontend: `frontend/src/components/Channel.tsx`, `frontend/src/playback.ts`, hls.js

## MediaMTX HLS sessions (1.18+)

MediaMTX now gates variant playlists (`main_stream.m3u8`) behind an HLS session:

1. First request to `index.m3u8` → **302** to `index.m3u8?cookieCheck=1` with `Set-Cookie` (`Secure; SameSite=None`).
2. Master playlist references `main_stream.m3u8?session=...`.
3. Variant/segment requests without a valid session → **401 Unauthorized**.

On **`http://localhost`**, browsers do not store `Secure` cookies, so playback fails with repeated 401s on `main_stream.m3u8` unless the proxy is recognized as a CDN.

## Local fix (required)

`deploy/mediamtx.yml`:

```yaml
hlsCDNSecret: streamclone-local-hls-cdn
```

Caddy injects the matching header on all `/live/*` reverse_proxy blocks:

```caddyfile
header_up Authorization "Bearer streamclone-local-hls-cdn"
```

Files: `deploy/Caddyfile.local-tunnel` (`@hls_local`, `@hls`) and `deploy/Caddyfile` (production `@hls`).

**Do not remove** this pairing without an alternative that works on plain HTTP localhost. Internal video-service HLS probes hit `mediamtx:8888` directly (server-side cookie jar) and can succeed while the browser still 401s through the proxy.

## Symptom guide

| Symptom | Likely cause |
|---------|----------------|
| 401 on `main_stream.m3u8` via `:8090` | Missing or mismatched `hlsCDNSecret` / Caddy Bearer |
| 302 loop on `index.m3u8?cookieCheck=1` | Same — session cookies never stick on HTTP |
| Transient 401s during `docker compose up --build` | Old HLS session URLs in an open tab; hard-refresh after stack stabilizes |
| 404 on segments | Stream not started or worker not publishing to MediaMTX yet |
| Playback works on LAN IP but not localhost | Stale `wslrelay` — see `.kiro/steering/windows-dev.md` |

## API flow

- `POST /v1/stream/start` → returns `hlsUrl` like `http://localhost:8090/live/{channel}/index.m3u8` (via `HLS_PUBLIC_BASE` / `PUBLIC_ORIGIN` in local-tunnel compose).
- `POST /v1/stream/keepalive` — session listener heartbeat; aborts during restarts are normal.
- Frontend rewrites `/live/` URLs to same origin via `normalizeBrowserOriginUrl` in `frontend/src/config.ts`.

## Task checklist

- Read this file and `.kiro/steering/tech.md` before HLS/proxy changes.
- Codegraph: `get_call_chain("waitForHLS")`, `get_blast_radius("proxyPlaylist")`.
- Verify after changes:
  - `curl -X POST http://localhost:8090/v1/stream/start -H "Content-Type: application/json" -d "{\"channel\":\"zerator\",\"quality\":\"720p60\"}"`
  - `curl -s -o NUL -w "%{http_code}" http://localhost:8090/live/zerator/main_stream.m3u8` → expect **200** while stream is active.
- Tunnel/ngrok caveats: `deploy/LOCAL_HTTPS_OAUTH.md`.
