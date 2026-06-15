# Clip Studio Steering

Clip Studio is optional local automation for Twitch clips, VOD exports, captions, templates, and vertical renders. It must not become a dependency of Core Watch.

## Boundaries

- Worker/API: `clipper/liveclipper/`
- UI: `frontend/src/components/ClipStudio.tsx` and `frontend/src/components/clipStudio/`
- Same-origin proxy: `/v1/clipper/*`
- State: clipper SQLite and local files by default

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
- `get_ast_chunk("_process")`
- `get_ast_chunk("prepare_emote_assets")`

## Checks

```sh
make clipper-test
cd frontend && npm run build
make security-scan
```
