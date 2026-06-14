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
| `:8090` unreachable / `local-proxy` exit 127 / Caddy mount error | `deploy/Caddyfile.local-tunnel` is a **directory** instead of a file (bad Docker bind). **Start Streamclone** runs `Repair-StreamcloneCaddyfileLocalTunnel` in `scripts/start-streamclone.ps1`; install overlay ships the file from GitHub commit SHA |

## Architecture boundaries

Go does not download HLS segments in-process. Segment fetch and transmux are delegated to external subprocesses; the orchestrator is the control plane.

| Layer | In-process (Go) | Subprocess / external |
|-------|-----------------|----------------------|
| Playback token + usher | `internal/video/token`, `internal/video/usher` | GQL + usher HTTP |
| Default relay (`streamlink`) | spawn + supervise | `streamlink --stdout` → pipe → `ffmpeg -c copy -f flv` → MediaMTX RTMP |
| Fallback relay (`direct_hls`) | `/v1/stream/proxy` manifest rewrite | single `ffmpeg` reading proxied usher URL |
| Local HLS output | readiness probe (`waitForHLS`, 100ms ticks) | MediaMTX (`1s` segments × 5) |
| Browser playback | — | hls.js |

**`direct_hls` proxy path:** `proxyPlaylist` fetches upstream manifests and runs `filterTwitchAdSegments` on every FFmpeg refresh. That rewriter strips Twitch stitched-ad `DATERANGE` blocks and related segments. Probes cap manifest reads at 4 KB; the proxy path uses `io.ReadAll` without a size cap — treat large LL-HLS manifests as a known memory risk.

**Process supervision:** `worker.Kill` uses process-group SIGKILL (Linux). `worker.Reconcile` scans `/proc` for orphan streamlink/ffmpeg at boot. Windows lacks the same PGID semantics.

**Startup breakdown:** `registry.StartupBreakdown` tracks `upstreamFetchMs`, `workerSpawnMs`, and `hlsReadyMs` for cold-start diagnosis.

## API flow

- `POST /v1/stream/start` accepts `latency_mode` (`instant` | `fast` | `stable`). Maps to streamlink `--hls-live-edge` **1 / 2 / 3** and is stored on `registry.Session` for diagnostics.
- `POST /v1/stream/start` → returns `hlsUrl` like `http://localhost:8090/live/{channel}/index.m3u8` (via `HLS_PUBLIC_BASE` / `PUBLIC_ORIGIN` in local-tunnel compose).
- `GET /v1/stream/diagnostics?channel=` returns real `latencyMode`, `liveEdge`, worker restart stats, and HLS probe — not hardcoded labels.
- `POST /v1/stream/keepalive` — session listener heartbeat; aborts during restarts are normal.
- Frontend sends `settings.playbackLatencyMode` on start; `playback.ts` auto-downgrades **instant → fast → stable** after stall/rebuffer thresholds (brief on-player notice).
- `/v1/stream/proxy` only allows `http(s)` URLs on `*.ttvnw.net` / `usher.ttvnw.net`.
- Worker crashes: `supervise()` rotates `streamlink` ↔ `direct_hls` with backoff and re-runs `waitForHLS`.
- Playback tokens: short-lived per-channel cache + single-flight in `internal/video/token`.
- Frontend rewrites `/live/` URLs to same origin via `normalizeBrowserOriginUrl` in `frontend/src/config.ts`.
- **Archived VOD relay:** `POST /v1/stream/vod/start` with `vod_id` / offset starts a Streamlink→ffmpeg worker publishing to `live/vod_{id}/index.m3u8` (same Caddy `@hls` path as live). Channel deep-link: `/c/{login}?vod={id}&offset=`; analytics adds `from=analytics&sid={stream_id}` for activity/chat replay. Keep Twitch as an explicit fallback action, not an automatic redirect.
- VOD mode must stay inside the channel workspace, show `VodModeControls`, publish playhead sync for matching analytics charts, and load `VodChatReplayPanel` when `sid` is present.
- Theater layout: opening player settings must not shrink the theater player. `Shrink`/theater toggle exits theater; settings only expand controls.

## Latency / resilience knobs

| Knob | Default | Effect |
|------|---------|--------|
| `STREAM_WORKER_BACKENDS` | `direct_hls,streamlink` | `direct_hls` first for ~2–4s relay start; `streamlink` first waits 15s probe timeout before fallback |
| `latency_mode` on start | `stable` if omitted | Coordinates streamlink live-edge with browser hls.js mode |
| `playbackLatencyMode` (UI) | `fast` | Browser buffer policy; may auto-downgrade on stalls |
| `STREAMLINK_HLS_LIVE_EDGE` | `2` | Fallback when start request omits `latency_mode` |
| `HLS_FAST_START_SEGMENT_COUNT` | unset | When set (>0) or `instant`/`fast` start: faster HLS readiness probe (0 stability window; optional variant skip) |
| `HLS_PROBE_TIMEOUT` | 15s | Max wait for local HLS ready |

FFmpeg workers (streamlink pipe + `direct_hls`) use reconnect flags: `rw_timeout`, `reconnect_on_network_error`, `reconnect_on_http_error`, `reconnect_max_retries`, `reconnect_delay_max`.

See `docs/hls-relay-buffer-latency.md` for the full latency budget.

## Task checklist

- Read this file and `.kiro/steering/tech.md` before HLS/proxy changes.
- Codegraph: `get_call_chain("waitForHLS")`, `get_blast_radius("proxyPlaylist")`.
- Verify after changes:
  - `curl -X POST http://localhost:8090/v1/stream/start -H "Content-Type: application/json" -d "{\"channel\":\"zerator\",\"quality\":\"720p60\"}"`
  - `curl -s -o NUL -w "%{http_code}" http://localhost:8090/live/zerator/main_stream.m3u8` → expect **200** while stream is active.
- Tunnel/ngrok caveats: `deploy/LOCAL_HTTPS_OAUTH.md`.
