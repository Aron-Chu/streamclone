---
name: streamclone-clipper-local
description: Set up and debug Streamclone live clipper and Clip Studio locally. Use for missing_scope, Helix clip failures, clipper jobs, or Clip Studio at /studio.
---

# Streamclone clipper local

Read `.kiro/steering/clipper.md` first.

## Guardrails

- Clipper logic stays in `clipper/liveclipper/` — not in Go viewer services
- UI surfaces: `ClipStudio.tsx`, `Analytics.tsx`, proxied at `/v1/clipper/*`
- Never put tokens in URLs, logs, or SQLite display fields

## Auth + scopes

**Recommended (no Twitch CLI):** open http://localhost:8090 → **Sign in (optional)**. That runs the device-code flow in the browser and syncs clipper credentials automatically.

Developers with Twitch CLI:

```powershell
make twitch-local-auth
# or
powershell -ExecutionPolicy Bypass -File scripts/twitch-auth.ps1 -Action local-auth -Scopes "chat:read chat:edit user:read:follows clips:edit" -ChatHttp "http://localhost:8090"
```

Both paths run `ensure-clipper-auth` + `ensure-frontend-config` so clipper and `/config.js` stay aligned with `.env` (plain `docker compose restart` is not enough).

Manual recovery:

```powershell
make ensure-clipper-auth
make ensure-frontend-config
```

## Diagnostics

1. MCP **`streamclone-stack`** → `twitch_auth_status`
2. MCP **`streamclone-stack`** → `compose_logs(service="clipper")`
3. `make clipper-test`

## Code lookup

**`streamclone-codegraph`**: `get_ast_chunk("ClipStudio")`, `get_call_chain("_process")`
