---
description: Work on Streamclone Clip Studio, live clipper worker/API, VOD exports, captions, templates, and clipper auth.
---

# Clipper Local

Read `AGENTS.md`, `.kiro/steering/clipper.md`, and `docs/security.md`.

## Guardrails

- Keep rendering and job state in `clipper/`.
- Keep Go viewer services as API consumers only.
- Do not log or expose Twitch tokens or webhook tokens.

## Codegraph

- `get_ast_chunk("ClipStudio")`
- `get_ast_chunk("VideoStage")`
- `get_ast_chunk("CaptionOverlayEditor")`
- `get_ast_chunk("_process")`

## Tests

```sh
make clipper-test
cd frontend && npm run build
make security-scan
```
