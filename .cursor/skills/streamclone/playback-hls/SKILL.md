---
name: streamclone-playback-hls
description: Debug Streamclone HLS playback and MediaMTX 401 errors through localhost:8090. Use for channel player failures, main_stream.m3u8 401, cookieCheck loops, or MediaMTX session issues.
---

# Streamclone playback / HLS

Read `.kiro/steering/playback.md` first.

## Config check (before runtime probes)

Verify pairing in repo:

- [`deploy/mediamtx.yml`](deploy/mediamtx.yml) → `hlsCDNSecret: streamclone-local-hls-cdn`
- [`deploy/Caddyfile.local-tunnel`](deploy/Caddyfile.local-tunnel) → `@hls_local` / `@hls` blocks include `header_up Authorization "Bearer streamclone-local-hls-cdn"`

Do not remove this pairing without an alternative that works on plain HTTP localhost.

## Runtime probes

1. MCP **`streamclone-stack`** → `playback_probe(channel="<login>")`
2. Interpret results:
   - **401 on `main_stream.m3u8`** → CDN secret / Bearer mismatch
   - **404 on segments** → stream not started; call `POST /v1/stream/start` first
   - **302 loop on `index.m3u8?cookieCheck=1`** → same CDN secret issue on HTTP localhost

## Code lookup

Use **`streamclone-codegraph`**: `get_call_chain("waitForHLS")`, `get_ast_chunk` on orchestrator handlers.

## User guidance

After stack restarts, tell the user to hard-refresh — old HLS session URLs in open tabs cause transient 401s.
