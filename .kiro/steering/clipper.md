# Clip Studio Steering

**Active Clip Studio lives in [ReplayForge](../replayforge)** (sibling repo). The in-repo `clipper/` stub and compose profile are **deprecated** — Streamclone proxies to `host.docker.internal:8095` when ReplayForge is running.

See [docs/agents-streamclone-and-replayforge.md](../../docs/agents-streamclone-and-replayforge.md) for install and integration.

## Boundaries

- Worker/API: ReplayForge backend (legacy reference: `clipper/liveclipper/`)
- UI: ReplayForge frontend at `:8096`; Streamclone routes `/studio` and links out
- Same-origin proxy: `/v1/clipper/*` → host ReplayForge API
- State: ReplayForge SQLite and local files

Do not move Helix writes, transcription, or FFmpeg rendering into Go viewer services.

## Current Flows

- Live path: Helix clip creation -> Streamlink download -> transcription -> render.
- Historical path: Analytics moment with `vod_id` -> VOD segment download -> trim/export.
- Mobile Studio uses panes instead of fixed desktop rails.

## Guardrails

- Keep tokens out of URLs, filenames, logs, and display strings.
- Validate webhook/mutation tokens.
- Use argument arrays for Streamlink and FFmpeg.
- Render without subtitles when transcript is empty.
- Keep one render worker by default.
- Preserve duplicate suppression and retention cleanup.

## Codegraph Hints

- `get_ast_chunk("ClipStudio")`
- `get_ast_chunk("VideoStage")`
- `get_ast_chunk("CaptionOverlayEditor")`
- ReplayForge: `_process` in ReplayForge backend

## Checks

```sh
# ReplayForge (when checked out as ../replayforge)
cd ../replayforge && make test

cd frontend && npm run build
make security-scan
```
