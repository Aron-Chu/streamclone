# Playback Steering

Playback is local HLS through Caddy and MediaMTX. The browser should use `http://localhost:8090`, not raw service ports.

## Boundaries

- Video service: `cmd/video`, `internal/video/*`
- Channel UI: `frontend/src/components/Channel.tsx`
- Proxy: `deploy/Caddyfile.local-tunnel`, `deploy/Caddyfile`
- MediaMTX: `deploy/mediamtx.yml`

## Rules

- MediaMTX HLS session cookies require matching `hlsCDNSecret` and Caddy `Authorization: Bearer` on `/live/*`.
- Keep requested quality separate from loaded quality in UI.
- Keep Streamlink/FFmpeg process cleanup reliable.
- VOD playback should surface honest errors for invalid, unavailable, auth-blocked, or warming streams.
- Analytics VOD review (Twitch embed + `from=analytics`): scrubbing lives in the **Pulse** sidebar activity chart and moments — not as overlay heatmap/seek bar on the embed player.
- **Analytics VOD review** (`from=analytics` + `sid`): Twitch embed is the primary player; scrubbing lives in the **Pulse** sidebar (`ActivityWaveform` + moments), not on the iframe overlay. Relay/HLS keeps the player heatmap and seek bar when embed is not used.

## Codegraph Hints

- `get_call_chain("waitForHLS")`
- `get_blast_radius("filterTwitchAdSegments")`
- `get_ast_chunk("Channel")`
- `get_ast_chunk("LivePlayerControls")`

## Checks

```sh
go test ./internal/video/...
cd frontend && npm run build
make compose-config-check
.\scripts\measure-hls-latency.ps1 -Channels mrekk,sodapoppin,thebausffs
```

For local validation, probe through `http://localhost:8090/live/{channel}/index.m3u8`.

## Latency tuning (2026-06)

- MediaMTX default live window: `hlsSegmentCount: 10` (20s sliding window at 2s segments); does not change origin edge delay — mainly limits drift after stalls.
- Browser mpegts sync targets: `instant` 2/3 segments (tighter buffer), `fast` 2/4, `stable` 4/6 (2s MediaMTX segments).
- `direct_hls` HLS readiness probe uses full `HLS_PROBE_TIMEOUT`; optional `HLS_DIRECT_PROBE_TIMEOUT_SEC`.
- LL-HLS and WebRTC remain flag-gated; see `docs/low-latency-relay/`.

Deploy after tuning:

```powershell
docker compose -f deploy/docker-compose.yml --profile core up -d --build --force-recreate mediamtx frontend video
```

Success criteria (Fast mode, healthy `direct_hls`):

- Active backend remains `direct_hls`
- `measuredDelaySec` ~4s
- hls.js `latencyToLiveSec` target ~4s
- Composed display delay ~7–9s (not ~10–12s)
- No repeated stall downgrade to Stable during 5–10 min watch
- MediaMTX manifest shows ~10 live segments (not 15)
