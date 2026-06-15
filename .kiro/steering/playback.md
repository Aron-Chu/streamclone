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
```

For local validation, probe through `http://localhost:8090/live/{channel}/index.m3u8`.
