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

## Release install sync

`%USERPROFILE%\streamclone` from Setup.exe / ZIP is a **release install**, not a git checkout. The product repo is [`Aron-Chu/streamclone`](https://github.com/Aron-Chu/streamclone); this workspace folder may still be named `twitch-7tv-clone` locally. Its running app is controlled by extracted `VERSION`, `.env` `IMAGE_TAG`, and Docker images. Source commits here do not affect that install until images are rebuilt/published and the install is updated.

**Install/bootstrap fixes ship from git:** `launchers/Install Streamclone.cmd` and **Start Streamclone** overlay scripts from the latest GitHub commit SHA (`Get-StreamcloneGitHubMasterSha` in `scripts/bootstrap-windows-install.ps1` — raw `/master/` CDN can lag). Log end-user fixes in `docs/repo-maintenance.md` (*Install bug fix log*).

Check drift:

```powershell
Get-Content C:\Users\Aron\streamclone\VERSION
Select-String '^IMAGE_TAG=' C:\Users\Aron\streamclone\.env
```

If they differ, run **Manage Streamclone → Update** or `Invoke-StreamcloneUpgrade` from `scripts\lib\install-upgrade.ps1`. Copying source files into the install folder only updates scripts/docs; it does not update Go/frontend code baked into images.

**Common install repairs (2026-06):**

| Issue | Fix in repo |
|-------|-------------|
| Bootstrap `%TEMP%\lib\env.ps1` not found | `$StreamcloneBootstrapLibDir` in `bootstrap-windows-install.ps1` |
| Stale bootstrap after push | Overlay fetched by commit SHA, not `/master/` raw URL |
| `:8090` down, `Caddyfile.local-tunnel` is a directory | `Repair-StreamcloneCaddyfileLocalTunnel` on start; overlay includes `deploy/Caddyfile.local-tunnel` |
| Uninstall fails when Docker offline | Deferred cleanup + `Finish Streamclone Docker cleanup.cmd` |

## Twitch local auth (Windows)

```powershell
make twitch-local-auth
# or
powershell -ExecutionPolicy Bypass -File scripts/twitch-auth.ps1 -Action local-auth -Scopes "chat:read chat:edit user:read:follows clips:edit" -ChatHttp "http://localhost:8090"
docker compose --env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml up -d --force-recreate clipper
```

## Setup-control (port 9191)

Host-side HTTP helper for install wizard, **Start Analytics**, and optional clipper start. Not a Docker container.

- **Started by:** `scripts/ensure-setup-control.ps1` (called from `setup.ps1`, `start-streamclone.ps1`, install launcher).
- **PID file:** `.streamclone-setup-control.pid` in repo root — written on start, removed on stop/uninstall (`scripts/stop-streamclone.ps1`, `scripts/uninstall-streamclone.ps1`).
- **Proxied:** Caddy `/v1/setup-control/*` → `host.docker.internal:9191`.

**Symptom:** **Start Analytics** or install wizard does nothing; UI shows `installHelperReady: false`.

**Check:**

```powershell
curl.exe http://127.0.0.1:9191/health
curl.exe http://localhost:8090/v1/setup-control/health
```

**Fix:** run **Start Streamclone** once, or:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/ensure-setup-control.ps1
```

If `.env` changed after setup-control started, `ensure-setup-control.ps1` restarts the daemon when the PID file is stale.

## Task checklist

- Read this file when localhost behaves differently from `docker ps` / container logs.
- Prefer `curl.exe http://localhost:8090/...` for probes on Windows PowerShell.
- If optional services or install wizard fail, check setup-control on `:9191` before blaming Docker.
- After `.env` / OAuth changes, run `make reload-env` (or `make twitch-sync`, which calls it). `docker compose restart` does **not** reload `env_file`; affected services: `chat`, `metadata`, `analytics`, `emote`, and `clipper` when using Clip Studio. `make app` / `make up` run `ensure-oauth` + `reload-env-if-stale` to catch drift (e.g. `.env` has OAuth but `emote` container was created without it).
- For scraper/full installs, `scripts\scraper-preflight.ps1 -CheckOnly` must pass sequential TwitchTracker detail probes; `/health` alone is not enough.
