---
name: streamclone-stack-debug
description: Diagnose Streamclone localhost issues (stale data, wslrelay, auth, proxy). Use when localhost:8090 behaves wrong, docker ps looks healthy, or the user mentions wslrelay, stale stack, or auth broken.
---

# Streamclone stack debug

Read `.kiro/steering/windows-dev.md` first.

## Workflow

1. Call MCP **`streamclone-stack`** → `stack_ports`
2. If port **8090** (or 8086) is owned by **`wslrelay`**:
   - `wsl --shutdown`
   - Recreate stack: `docker compose --env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml up -d`
3. Call **`stack_health`** — review `auth_debug`, `warnings`, and `containers`
4. If auth/session issues → **`twitch_auth_status`**
5. If HLS/player issues → switch to **`streamclone-playback-hls`** skill
6. If analytics/scraper issues → **`streamclone-analytics-sync`** skill

## UI verification

Use **Playwright MCP** only against **`http://localhost:8090`** (Caddy proxy). Do not point Playwright at `:5174`, `:8081`, or raw service ports unless intentionally bypassing the proxy.

## Probes

- Auth: `curl.exe http://localhost:8090/v1/auth/debug`
- Ports: `powershell -File scripts/stack-ports.ps1`
- After `.env` / OAuth changes: `make reload-env` (chat, metadata, analytics, emote) or `make twitch-sync`. `make app` auto-runs `reload-env-if-stale` when container OAuth ≠ `.env`.

## Output

Summarize: port owners, auth warnings, unhealthy containers, and the single next fix — not full JSON dumps.
