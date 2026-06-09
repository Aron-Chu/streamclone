# HLS relay, buffering, and Twitch latency

How Streamclone relays Twitch live video, how the browser buffers it, and how we measure delay versus native Twitch playback.

**Related code:** `internal/video/orchestrator`, `internal/video/worker`, `deploy/mediamtx.yml`, `frontend/src/playback.ts`, `frontend/src/playbackMath.ts`, `frontend/src/components/PlaybackDiagnostics.tsx`.

**Steering:** `.kiro/steering/playback.md`, `memories/repo/playback-notes.md`.

---

## End-to-end path

Streamclone does **not** point the browser at Twitch HLS directly. We run a local relay:

```
Twitch (Usher HLS)
    ↓  streamlink or ffmpeg
RTMP publish → MediaMTX (:1935)
    ↓  re-segment as local HLS
MediaMTX HLS (:8888)  /live/{channel}/index.m3u8
    ↓  Caddy proxy (:8090)
Browser (hls.js → <video>)
```

Each hop adds latency. Total “behind live” is the sum of:

1. **Upstream ingest lag** — how far behind Twitch’s live edge streamlink/ffmpeg reads.
2. **MediaMTX segmentization** — 1s segments, 5-segment sliding window.
3. **Browser buffer + hls.js live sync** — controlled by the user’s latency mode (`stable` / `fast` / `instant`).

Chat and metadata are separate paths (IRC WebSocket, Helix). They are **not** time-aligned with the HLS relay unless you manually compare timestamps.

---

## Phase 1: Starting the relay (`POST /v1/stream/start`)

Implemented in `internal/video/orchestrator/orchestrator.go` (`create`, `startWorker`, `waitForHLS`).

### 1. Upstream discovery (`upstreamFetchMs`)

| Step | API | Purpose |
|------|-----|---------|
| Playback token | Twitch GQL / token client | Signed access for the channel |
| Usher master playlist | `usher.Client.Discover` → `https://usher.ttvnw.net/api/channel/hls/{login}.m3u8` | Lists renditions (720p60, source, audio_only, …) |
| Rendition selection | `selectRendition(rends, quality)` | Maps UI quality preset to one rendition URL |

`upstreamFetchMs` = wall time from start of upstream work through rendition selection.

### 2. Worker spawn (`workerSpawnMs`)

Two backends, tried in order (`STREAM_WORKER_BACKENDS`, default `streamlink,direct_hls`):

#### A. `streamlink` (default)

`internal/video/worker/worker.go` — pipe architecture:

```
streamlink twitch.tv/{channel} {quality} --stdout
    → ffmpeg -i pipe:0 -c copy -f flv rtmp://mediamtx:1935/live/{channel}
```

Important streamlink flags:

| Flag | Effect on latency |
|------|-------------------|
| `--twitch-low-latency` | Requests Twitch low-latency HLS behavior |
| `--hls-live-edge N` | Stay **N segments** behind Twitch’s live edge (`STREAMLINK_HLS_LIVE_EDGE`, default `2`; use `1` for minimum relay lag) |
| `--twitch-disable-ads` | Skips ad segments where possible |

`workerSpawnMs` = time to `exec` streamlink + ffmpeg and return (processes are running; segments may not exist yet).

#### B. `direct_hls` (fallback)

If streamlink fails or HLS never becomes ready, orchestrator tries FFmpeg reading the **Usher rendition URL** directly:

```
ffmpeg -reconnect 1 -i http://127.0.0.1:8080/v1/stream/proxy?url={usher_url} -c copy -f flv rtmp://...
```

`/v1/stream/proxy` rewrites the upstream media playlist and **strips Twitch stitched-ad segments** (`filterTwitchAdSegments`) so FFmpeg does not ingest ad windows.

### 3. HLS readiness probe (`hlsReadyMs`)

After the worker publishes RTMP, orchestrator polls MediaMTX until playlists are fetchable:

- URL: `{HLS_PROBE_BASE}/live/{channel}/index.m3u8` (server-side, usually `http://mediamtx:8888`)
- Interval: **100ms** (`hlsProbeInterval`)
- Stability window: playlist must stay valid for **100ms** (`hlsStabilityWindow`)
- Timeout: `HLS_PROBE_TIMEOUT` (default **15s**)

`waitForHLS` checks master `index.m3u8`, follows the first variant (`main_stream.m3u8`), and requires that child playlist to return 200.

`hlsReadyMs` = time spent in this probe loop.

### 4. Total relay startup (`startupMs`)

```text
startupMs = upstreamFetchMs + workerSpawnMs + hlsReadyMs
```

Returned in `POST /v1/stream/start` and `GET /v1/stream/diagnostics`. Shown in the Channel startup overlay as **Relay**, with breakdown **Up / Spawn / HLS**.

Typical local baseline (jynxzi, 2026-06-09): ~8.6s total, with **HLS ready** dominating (~8s) — waiting for enough segments to land in MediaMTX.

### Session lifecycle

| Endpoint | Role |
|----------|------|
| `POST /v1/stream/keepalive` | Browser heartbeat; prevents idle reaper from killing the worker |
| `POST /v1/stream/stop` | Tear down listener / stream |
| `supervise()` goroutine | Restarts worker on crash (up to `MAX_RESTARTS`, default 3) |

---

## Phase 2: MediaMTX HLS edge buffer

Config: `deploy/mediamtx.yml`

```yaml
hlsSegmentDuration: 1s
hlsSegmentCount: 5
hlsVariant: mpegts
```

MediaMTX keeps a **rolling window of 5 × 1s MPEG-TS segments** per path `live/{channel}`.

Implications:

- The browser can seek within roughly **~5 seconds** of buffered relay output.
- New viewers always join a playlist that is already a few seconds behind the RTMP ingest point.
- There is **no LL-HLS** (partial segments) on this edge — standard HLS latency physics apply.

### Localhost / session cookies

MediaMTX 1.18+ uses HLS sessions (`index.m3u8` → 302 → `main_stream.m3u8?session=...`). `Secure` cookies break on `http://localhost`.

**Fix:** `hlsCDNSecret` on MediaMTX + Caddy sends `Authorization: Bearer streamclone-local-hls-cdn` on `/live/*` so the browser gets 200 without cookie stickiness.

Browser URL (local tunnel compose): `http://localhost:8090/live/{channel}/index.m3u8` (`HLS_PUBLIC_BASE`).

---

## Phase 3: Browser load and buffer (hls.js)

Implemented in `frontend/src/playback.ts` (`useHlsPlayback`).

### Attach sequence

1. `hls.loadSource(hlsUrl)` + `attachMedia(video)`
2. Events: `MEDIA_ATTACHED` → `MANIFEST_PARSED` → `BUFFER_APPENDED` → first `playing` / `requestVideoFrameCallback` → **`firstFrameMs`**

`firstFrameMs` = `performance.now()` at first frame minus attach start. This is **browser-side** time after the relay URL is known — includes manifest fetch, segment download, decode, and hls.js live sync positioning.

### Latency modes (user setting)

`frontend/src/settings.ts` — `playbackLatencyMode`: `stable` (default in code comments but UI default is `fast`), `fast`, `instant`.

Each mode maps to hls.js config in `hlsLatencyConfig()`:

| Mode | `liveSyncDurationCount` | `maxBufferLength` | `lowLatencyMode` | Intent |
|------|-------------------------|-------------------|------------------|--------|
| **instant** | 1 | 2s | yes | Minimum buffer; ~1 segment behind sync point |
| **fast** | 1.5 | 4s | yes | Default UX — low lag, small cushion |
| **stable** | 5 | 15s | no | Larger buffer, fewer stalls on jittery networks |

Also: `maxLiveSyncPlaybackRate` (1.0–1.3) lets hls.js speed up slightly to catch the live edge in fast/instant modes.

### Buffer size metric (`bufferSizeSec`)

```typescript
// playback.ts — bufferedAhead()
bufferSizeSec = buffered.end(i) - video.currentTime
```

For the buffer range containing `currentTime`, this is **seconds of media already downloaded ahead of the playhead**. It is **not** total buffered ranges or relay segment count.

### Live edge metrics (local)

Computed in `getLiveEdgeMetrics()` + `calculateLiveEdge()` (`playbackMath.ts`):

| Metric | Source | Meaning |
|--------|--------|---------|
| `currentTimeSec` | `video.currentTime` | Playhead position in the MediaMTX playlist timeline |
| `liveSyncPositionSec` | `hls.liveSyncPosition` | Where hls.js wants the playhead for live sync |
| `seekableEndSec` | `video.seekable.end(last)` | End of seekable range (= latest buffered segment end) |
| `targetLatencySec` | `hls.targetLatency` | hls.js target offset from live edge |
| `behindLiveSec` | `max(0, referenceEnd - currentTime)` | **Primary “how far behind live”** local metric; `referenceEnd = liveSyncPosition ?? seekableEnd` |
| `latencyToLiveSec` | `hls.latency` | hls.js internal latency estimate (when exposed) |

**Jump Live** (`jumpLive`): sets `currentTime = liveSyncPosition` (or `seekableEnd - targetLatency`) when `behindLiveSec > max(targetLatency + 4, 10)`.

Diagnostics panel labels:

- **Live latency** → `latencyToLiveSec` (hls.js)
- **Behind live** → `behindLiveSec` (our calculation)
- **Target** → `targetLatencySec`

---

## Phase 4: Delay versus actual Twitch

Streamclone’s main player **does not** know Twitch’s native latency automatically. `readPlaybackMetrics()` sets `actualTwitchLatencySec` and `delayVsTwitchSec` to `null` until comparison UI fills them in.

### Comparison UI (`PlaybackDiagnostics.tsx`)

Two ways to get **Twitch reference latency**:

#### 1. Hidden Twitch embed (automatic)

When user clicks **Compare Twitch → Start ref**:

- Loads `https://player.twitch.tv/js/embed/v1.js`
- Creates a 1×1 hidden `Twitch.Player` on the same channel (muted)
- Polls every 2s: `player.getPlaybackStats().hlsLatencyBroadcaster`
- Stores as `referenceLatencySec` → displayed as **Twitch delay**

This is Twitch’s own “latency to broadcaster” stat from their embed player (same family of number as Twitch’s stats overlay).

#### 2. Manual paste

User can paste values from Twitch’s official stats for the channel into localStorage-backed fields, including **Latency sec** (`latencyToBroadcasterSec`).

### Delay calculation

```typescript
localLiveLatencySec = metrics.latencyToLiveSec ?? metrics.behindLiveSec

actualTwitchLatencySec = referenceLatency ?? manualLatency

delayVsTwitchSec = localLiveLatencySec - actualTwitchLatencySec
```

Displayed as **Local minus Twitch** / **Delay**:

| Value | Meaning |
|-------|---------|
| `+3.5s` | Streamclone is **3.5s more delayed** than Twitch’s native player |
| `-1.2s` | Streamclone is **1.2s ahead** of Twitch embed reference (unusual; may be measurement timing) |
| `-` | Missing local or Twitch reference |

`delayDelta()` formats with 2 decimal places when |diff| &lt; 10s.

**Copy JSON** exports a benchmark blob: local metrics, relay diagnostics, Twitch reference, `capturedAt`.

### What this does *not* measure

- **Chat delay** relative to video (IRC path is independent).
- **Frame-accurate A/V sync** to Twitch CDN (we compare player-reported latency stats, not cross-player PTS).
- **Relay-only lag** in isolation — local metrics include both relay pipeline and browser buffer policy.

---

## Latency budget (conceptual)

```text
Total viewer delay ≈
  streamlink live-edge (N × ~2s Twitch segments upstream)
+ ffmpeg + RTMP mux (small)
+ MediaMTX segmentization (up to ~1s per segment boundary)
+ MediaMTX 5-segment window (playlist depth)
+ network: Caddy → browser
+ hls.js target latency (mode-dependent, ~1–15s buffer policy)
```

Rough expectations:

| Knob | Direction |
|------|-----------|
| `STREAMLINK_HLS_LIVE_EDGE=1` | Less relay lag; more upstream stall risk |
| `playbackLatencyMode=instant` | Smaller browser buffer; more rebuffer risk |
| `playbackLatencyMode=stable` | Higher delay; smoother playback |
| Higher rendition (source / 1080p60) | Slower startup, similar live-edge physics |

---

## Observability cheat sheet

| UI / API field | Layer | What it measures |
|----------------|-------|------------------|
| `startupBreakdown.upstreamFetchMs` | Orchestrator | Token + Usher |
| `startupBreakdown.workerSpawnMs` | Worker | Process spawn |
| `startupBreakdown.hlsReadyMs` | Orchestrator | MediaMTX playlist probe |
| `startupMs` / Relay | API + overlay | Sum of relay bootstrap |
| `firstFrameMs` / Frame | Browser | Manifest + first decoded frame |
| `bufferSizeSec` | Browser | Downloaded media ahead of playhead |
| `behindLiveSec` | Browser | Offset from live sync position |
| `latencyToLiveSec` | hls.js | Internal live latency |
| `hlsLatencyBroadcaster` | Twitch embed | Twitch’s reference latency |
| `delayVsTwitchSec` | Diagnostics | Local minus Twitch |

---

## Configuration reference

| Variable | Default | Effect |
|----------|---------|--------|
| `STREAM_WORKER_BACKENDS` | `streamlink,direct_hls` | Relay backend order |
| `STREAMLINK_HLS_LIVE_EDGE` | `2` | Segments behind Twitch live edge (worker) |
| `MEDIAMTX_RTMP` | `mediamtx:1935` | RTMP ingest target |
| `HLS_PUBLIC_BASE` | `http://localhost:8090` (tunnel compose) | URL returned to browser |
| `HLS_PROBE_TIMEOUT` | 15s | Max wait for local HLS ready |
| `playbackLatencyMode` | `fast` (UI settings) | hls.js buffer / sync policy |

Benchmark script: `scripts/benchmark-hls-start.ps1 -Channel {login}`.

---

## Diagram

```mermaid
sequenceDiagram
  participant Browser
  participant Caddy
  participant Video as Video Orchestrator
  participant Twitch
  participant Worker as streamlink/ffmpeg
  participant MTX as MediaMTX

  Browser->>Video: POST /v1/stream/start
  Video->>Twitch: Playback token + Usher master
  Twitch-->>Video: Rendition URLs
  Video->>Worker: spawn (streamlink | direct_hls)
  Worker->>Twitch: HLS segments (live-edge N)
  Worker->>MTX: RTMP publish live/{channel}
  Video->>MTX: probe index.m3u8 (waitForHLS)
  MTX-->>Video: playlists ready
  Video-->>Browser: hlsUrl, startupMs, breakdown

  Browser->>Caddy: GET /live/{channel}/index.m3u8
  Caddy->>MTX: Authorization Bearer CDN secret
  MTX-->>Browser: HLS segments (5x1s window)
  Note over Browser: hls.js buffers per latency mode<br/>behindLiveSec, firstFrameMs

  opt Compare Twitch
    Browser->>Browser: Hidden Twitch.Player embed
    Note over Browser: hlsLatencyBroadcaster vs behindLiveSec<br/>= delayVsTwitchSec
  end
```
