# Windows Dev Steering

## Purpose

Streamclone is developed on Windows with Docker Desktop and often WSL2. Localhost routing and Python venv paths behave differently than on Linux — this file prevents repeated rediscovery.

## Canonical local URL

- **Use `http://localhost:8090`** for browser, curl, OAuth, chat WebSocket, and HLS.
- Frontend runtime `VITE_*_URL` values should stay **`auto`** (same-origin proxy) in `deploy/docker-compose.yml`.
- Do not point the browser at `5174`, `8081`, `8086`, `8095`, etc. unless intentionally bypassing the proxy.

## Stale localhost / wslrelay

**Symptom:** API returns old data, wrong tokens, empty rollups, or clipper `missing_scope` despite a fresh `.env` and recreated containers.

**Cause:** `wslrelay.exe` (or similar) binding `127.0.0.1` on service ports and forwarding to an **old** backend.

**Check:**

```powershell
netstat -ano | findstr ":8090"
netstat -ano | findstr ":8086"
```

If `wslrelay` owns the port, Docker may still be healthy on `0.0.0.0` or the LAN IP while localhost is wrong.

**Fix:**

```powershell
wsl --shutdown
docker compose --env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml up -d
```

Verify against `http://localhost:8090`, not raw container ports.

## Python / codegraph

- `.codegraph/.venv` is a **WSL/Linux** venv (`bin/python`, not `Scripts\python.exe`).
- Run `make codegraph` from WSL. Cursor MCP on Windows uses `wsl.exe --cd ${workspaceFolder} bash scripts/*-mcp.sh` (not `.cmd` or PowerShell).
- The Kuzu graph is a **file** at `.codegraph/streamclone.kuzu` (not a directory).
- Clipper tests: `make clipper-test` (needs Python in PATH or WSL).

## Cursor MCP (Windows)

Cursor MCP on Windows uses `wsl.exe` with `scripts/*-mcp.sh` (see `.cursor/mcp.json`). Do **not** use `.cmd` batch launchers, PowerShell wrappers, or nested `bash -lc`/`wslpath` in `mcp.json` — `.cmd` breaks MCP stdio on Windows; nested bash quoting fails in JSON.

Setup once:

```powershell
wsl bash -lc "cd /mnt/c/Users/Aron/twitch-7tv-clone && make codegraph-install && make codegraph"
powershell -ExecutionPolicy Bypass -File scripts/mcp-preflight.ps1
```

If MCP servers show red in Cursor Settings → MCP:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/verify-mcp-stdio.ps1
powershell -ExecutionPolicy Bypass -File scripts/mcp-preflight.ps1
```

Then reload the window. Enable: `streamclone-codegraph`, `streamclone-stack`, `streamclone-data` (optional: `playwright`).

## Docker commands

```powershell
docker compose --env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml up -d --build
docker compose --env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml logs -f clipper analytics
```

## Twitch local auth (Windows)

```powershell
make twitch-local-auth
# or
powershell -ExecutionPolicy Bypass -File scripts/twitch-auth.ps1 -Action local-auth -Scopes "chat:read chat:edit user:read:follows clips:edit" -ChatHttp "http://localhost:8090"
docker compose --env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml up -d --force-recreate clipper
```

## Task checklist

- Read this file when localhost behaves differently from `docker ps` / container logs.
- Prefer `curl.exe http://localhost:8090/...` for probes on Windows PowerShell.
- After `.env` token changes, recreate affected services (`clipper`, `chat`, `metadata`) — not just restart the browser.
