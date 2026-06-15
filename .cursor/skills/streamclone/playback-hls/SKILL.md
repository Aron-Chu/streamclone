---
description: Debug or change Streamclone channel playback, HLS, MediaMTX, Caddy proxying, or VOD relay behavior.
---

# Playback / HLS

Read `AGENTS.md`, `.kiro/steering/playback.md`, and `.kiro/steering/tech.md`.

## First Checks

- Validate through `http://localhost:8090`.
- Check Caddy `Authorization: Bearer` for `/live/*`.
- Check MediaMTX `hlsCDNSecret`.

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
