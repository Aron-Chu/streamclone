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

Required scopes include `clips:edit` (and chat scopes for IRC monitor).

```powershell
make twitch-local-auth
# or
powershell -ExecutionPolicy Bypass -File scripts/twitch-auth.ps1 -Action local-auth -Scopes "chat:read chat:edit user:read:follows clips:edit" -ChatHttp "http://localhost:8090"
docker compose --env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml up -d --force-recreate clipper
```

## Diagnostics

1. MCP **`streamclone-stack`** → `twitch_auth_status`
2. MCP **`streamclone-stack`** → `compose_logs(service="clipper")`
3. `make clipper-test`

## Code lookup

**`streamclone-codegraph`**: `get_ast_chunk("ClipStudio")`, `get_call_chain("_process")`
