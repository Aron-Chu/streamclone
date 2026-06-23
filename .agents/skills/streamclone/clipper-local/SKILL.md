---
description: Work on Streamclone Clip Studio, live clipper worker/API, VOD exports, captions, templates, and clipper auth.
---

# Clipper Local

Read `AGENTS.md`, `.kiro/steering/clipper.md`, and `docs/security.md`.

## Guardrails

- **Active Clip Studio lives in the ReplayForge sibling repo** (`../replayforge` on host `:8095` API, `:8096` UI). The in-repo `clipper/` stub is deprecated — used only for `make clipper-test` fallback.
- Keep Go viewer services as API consumers only (proxy `/v1/clipper/*` to ReplayForge).
- Do not log or expose Twitch tokens or webhook tokens.

## Codegraph

- `get_ast_chunk("ClipStudio")`
- `get_ast_chunk("VideoStage")`
- `get_ast_chunk("CaptionOverlayEditor")`
- `get_ast_chunk("_process")` (ReplayForge backend when sibling checkout exists)

## Tests

```sh
make clipper-test
cd frontend && npm run build
make security-scan
```
