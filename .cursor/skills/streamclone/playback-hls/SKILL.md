---
description: Debug or change Streamclone channel playback, HLS, MediaMTX, Caddy proxying, or VOD relay behavior.
---

# Playback / HLS

Read `AGENTS.md`, **`.kiro/steering/playback.md`** (required), and `.kiro/steering/tech.md`. Requirements: `docs/low-latency-relay/requirements.md`.

## Separate latency layers

Diagnose **upstream**, **relay**, and **browser** independently — do not lump "HLS is slow" into one fix:

| Layer | What it is | Where to look |
|-------|------------|---------------|
| **Twitch upstream** | Source stream, ad segments, CDN | Video service relay start, `filterTwitchAdSegments` |
| **Relay (MediaMTX / Caddy)** | Local HLS packaging, CDN secret, proxy routes | `deploy/mediamtx.yml`, Caddy `/live/*`, `internal/video` |
| **Browser (hls.js buffer)** | Player buffer, `waitForHLS`, UI latency mode | `frontend/src/playback.ts`, channel player components |

## First checks

- Validate through `http://localhost:8090` — never raw MediaMTX port unless intentional.
- Check Caddy `Authorization: Bearer` for `/live/*`.
- Check MediaMTX `hlsCDNSecret` — mismatch → HLS 401.

## MCP / probes

- `playback_probe(channel)` — manifest + stream diagnostics via Caddy
- `stack_health` — video service health
- `compose_logs("mediamtx")`, `compose_logs("video")`, `compose_logs("local-proxy")`

## Latency benchmark

```powershell
.\scripts\measure-hls-latency.ps1
```

Compare upstream vs relay vs browser segments from the script output before tuning compose env.

## Codegraph

- `get_call_chain("waitForHLS")`
- `get_blast_radius("filterTwitchAdSegments")`
- `get_ast_chunk("Channel")`

## Tests

```sh
go test ./internal/video/...
cd frontend && npm run build
make compose-config-check
```
