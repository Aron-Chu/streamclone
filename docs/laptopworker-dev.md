# Laptopworker dev hub

Tailnet host **`laptopworker`** runs the **core Streamclone dev stack** (UI, playback, chat, emotes, local Postgres). **Network-heavy work stays on BearHost VPS** (scraper, corpus workers, silver/gold backfill).

## Quick control (Windows)

From the **streamclone repo root** (not `C:\Windows\System32`):

```powershell
make laptopworker-status
make laptopworker-smoke
make laptopworker-update
```

Or without `make`:

```cmd
scripts\laptopworker-remote.cmd status
scripts\laptopworker-remote.cmd smoke
scripts\laptopworker-remote.cmd update
```

PowerShell directly:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\laptopworker-remote.ps1 status
```

Commands on laptop: `bash scripts/laptopworker-stack.sh` → `start` | `stop` | `restart` | `status` | `logs` | `smoke` | `update` | `install-service` | `ufw-tailnet`

Browse UI on tailnet: **http://laptopworker:8090**

---

## Workload split

| Host | Runs | Does not run |
|------|------|--------------|
| **BearHost VPS** | `scraper`, `analytics-workers`, corpus/silver/gold, TwitchTracker/Camoufox | — |
| **laptopworker** | Caddy `:8090`, frontend, metadata, video, chat, emote, analytics API, postgres, redis, minio, mediamtx | Local scraper profile, storygraph ingest, corpus workers |
| **Windows PC** | Cursor, `laptopworker-remote.*` | — |

Env: `deploy/env/profile-laptopworker-dev.env` merged into `.env.local` (**user keys win**; bootstrap/update never clobber custom overrides).

Compose overlay: `deploy/docker-compose.laptopworker-dev.yml` (disables `storygraph`)

Shared helpers: `scripts/laptopworker-env.sh`

---

## Operator checklist (Aron)

One-time on laptop:

```bash
ssh aron@laptopworker
cd ~/streamclone
bash scripts/laptopworker-install-service.sh
sudo loginctl enable-linger aron
bash scripts/laptopworker-stack.sh ufw-tailnet   # tailnet-only :8090 + SSH
sudo reboot
```

After reboot (from Windows repo root):

```cmd
scripts\laptopworker-remote.cmd smoke
```

After every push to `master`:

```cmd
scripts\laptopworker-remote.cmd update
```

Optional Sign-in (only if needed):

```powershell
scp deploy\env\oauth-bundle.env aron@laptopworker:~/streamclone/deploy/env/
```

Add Twitch OAuth redirect: `http://laptopworker:8090` (and exact callback path if your app requires it). Never commit OAuth files.

---

## Update flow (P1)

`scripts/laptopworker-update.sh`:

1. `git fetch` + fast-forward `master`
2. Merge `.env.local` (preserve overrides) + resynth `.env`
3. Detect changed paths between old/new SHA
4. Rebuild only affected services:

| Changed paths | Action |
|---------------|--------|
| `go.mod`, `go.sum` | Rebuild all Go services |
| `frontend/**`, `packages/pulse-core/**` | Rebuild `frontend` |
| `cmd/**`, `internal/**`, `deploy/Dockerfile*` | Rebuild Go services |
| `deploy/docker-compose*`, `profile-laptopworker*` | Full `compose up --build` |
| `deploy/Caddyfile*` | Rebuild `frontend`; recreate `local-proxy` |
| `deploy/mediamtx.yml` | Recreate `mediamtx`, `video` |
| `migrations/**` | Run `migrate` container, then rebuild Go services |
| Docs-only / other | `compose up -d` without `--build` |

5. Smoke via `:8090`

---

## Tailscale dev efficiency

| Item | Value |
|------|-------|
| MagicDNS UI | `http://laptopworker:8090` |
| SSH | `ssh aron@laptopworker` (Tailscale SSH) |
| Subnet routes / exit node | **Off by default** — laptop is a private dev host, not a router |
| Key expiry | Consider disabling for this trusted always-on node in Tailscale admin |
| Production-like API | Use **BearHost** `https://api.streampulse.stream` for public ingress tests |
| VPS-only services | BearHost must appear in `tailscale status` only if laptop needs direct tailnet access to VPS |

---

## Security (tailnet-only)

Defense in depth (UFW alone does not filter Docker-published ports):

1. **DOCKER-USER** — `scripts/laptopworker-ufw-tailnet.sh` allows `:8090` on `lo` (local smoke) and `tailscale0` only; drops other interfaces.
2. **UFW** — SSH allowed on `tailscale0` only by default (set `LAPTOPWORKER_UFW_ALLOW_LAN_SSH=1` for LAN console SSH).

Docker still publishes `8090:80` on all interfaces (same as `local-tunnel`); DOCKER-USER is what blocks home-LAN access. Tailscale-IP compose bind is not used — it fails on some Docker/Tailscale hosts.

```bash
bash scripts/laptopworker-stack.sh ufw-tailnet
sudo ufw status verbose
sudo iptables -S DOCKER-USER | head -6
```

Docker service ports (`5432`, `6379`, etc.) stay on `127.0.0.1` in compose — never publish them on `0.0.0.0`.

---

## Systemd boot reliability

Install once:

```bash
bash scripts/laptopworker-install-service.sh
sudo loginctl enable-linger aron
```

| Unit | Role |
|------|------|
| `streamclone-dev.service` | Start stack on boot (user unit, `sg docker`) |
| `streamclone-dev-health.timer` | Smoke every 10 min |

Logs:

```bash
journalctl --user -u streamclone-dev.service -n 50 --no-pager
journalctl --user -u streamclone-dev-health.service -n 20 --no-pager
loginctl show-user aron -p Linger
systemctl --user status streamclone-dev.service
```

Reboot test: `sudo reboot` → wait ~3–5 min → `scripts\laptopworker-remote.cmd smoke` from Windows.

Future option: system-level unit with `User=aron` / `Group=docker` if user service proves fragile.

---

## Docker disk hygiene

Check usage:

```bash
docker system df
```

Safe manual cleanup (does not remove running stack volumes):

```bash
docker builder prune
```

Avoid `docker system prune -a` unless you intend to re-pull/rebuild everything.

---

## OAuth / Sign-in (optional)

| Step | Action |
|------|--------|
| 1 | Copy `deploy/env/oauth-bundle.env` to laptop (not committed) |
| 2 | Re-run update or `laptopworker_synth_env` path via `bash scripts/laptopworker-update.sh` |
| 3 | Add Twitch redirect URI: `http://laptopworker:8090` |
| 4 | Sign in via UI at `http://laptopworker:8090` |

Secrets dir: `~/.streamclone/secrets/` (mode 700)

---

## What the laptop hosts (core stack)

### Docker services

| Service | Purpose | Host port |
|---------|---------|-----------|
| `local-proxy` (Caddy) | Single entry `:8090` | `8090:80` (DOCKER-USER: lo + tailscale0 only) |
| `frontend` | React UI | internal |
| `metadata` | Directory, Helix, VOD lists | `127.0.0.1:8081` |
| `video` | HLS relay, streamlink | `127.0.0.1:8082` |
| `chat` | IRC bridge, Twitch OAuth | `127.0.0.1:8083` |
| `emote` | 7TV/FFZ pipeline | `127.0.0.1:8084` |
| `analytics` | Pulse BFF, extension routes | `127.0.0.1:8086` |
| `postgres` | Local dev DB | `127.0.0.1:5432` |
| `redis` | Cache | `127.0.0.1:6379` |
| `minio` | Emote storage | `127.0.0.1:9000` |
| `mediamtx` | RTMP/HLS | `127.0.0.1:8888` |

### Host packages

Docker, git, curl, make, jq, Tailscale — no Go/Node on host.

### Power (always-on)

`scripts/laptopworker-power-config.sh`: lid ignore, idle ignore, sleep targets masked. Keep AC connected.

---

## Bootstrap (fresh laptop)

```bash
ssh aron@laptopworker
git clone https://github.com/Aron-Chu/streamclone.git ~/streamclone
cd ~/streamclone
bash scripts/laptopworker-bootstrap.sh
bash scripts/laptopworker-install-service.sh
sudo loginctl enable-linger aron
bash scripts/laptopworker-stack.sh ufw-tailnet
```

---

## Health checks

```bash
bash scripts/laptopworker-stack.sh smoke
curl -fsS http://127.0.0.1:8090/v1/extension/health
```

Compose config sanity:

```bash
docker compose --env-file .env --env-file .env.local \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.local-tunnel.yml \
  -f deploy/docker-compose.laptopworker-dev.yml \
  config >/dev/null && echo OK
```

---

## Dev workflows (extension, portal, Remote SSH)

### Backend targets

| Mode | Backend URL | When to use |
|------|-------------|-------------|
| Windows local stack | `http://localhost:8090` | Daily dev on PC with `make up` |
| Laptop dev hub | `http://laptopworker:8090` | Shared tailnet stack; lighter on PC RAM |
| Production / BearHost | `https://api.streampulse.stream` | Public ingress, scrape/corpus, hosted API tests |

Health check (all modes):

```bash
curl -fsS http://laptopworker:8090/v1/extension/health
# Windows local: curl -fsS http://localhost:8090/v1/extension/health
```

### StreamPulse extension (sibling `streamclone-pulse`)

Config lives in **Chrome extension storage**, not committed env files:

| Item | Location |
|------|----------|
| Backend URL key | `backendUrl` in `chrome.storage.sync` |
| Default | `http://localhost:8090` (`src/shared/storage.ts`) |
| UI to change | Extension **Options** page (`src/options/options.tsx` → Backend URL) |
| Popup readout | `src/popup/popup.tsx` |

Switch to laptop hub:

1. Confirm smoke: `scripts\laptopworker-remote.cmd smoke`
2. Open extension **Options** → set Backend URL to `http://laptopworker:8090` → Save
3. Reload a Twitch channel tab

Do **not** commit OAuth tokens or machine-specific URLs into the repo. For production API tests, use `https://api.streampulse.stream` in Options only when you intend to hit BearHost (extension still talks to `/v1/extension/*` on that host).

**CORS / origins:** laptop profile sets `PUBLIC_ORIGIN`, `FRONTEND_ORIGIN`, and `HLS_PUBLIC_BASE` to `http://laptopworker:8090` (`deploy/env/profile-laptopworker-dev.env`). Twitch pages load from `https://www.twitch.tv`; the BFF must allow extension origins — same as localhost dev when origins are merged into `.env.local`.

**OAuth caveat:** Sign-in on the laptop UI requires a Twitch redirect for `http://laptopworker:8090` in the Twitch dev console (optional; see [OAuth / Sign-in](#oauth--sign-in-optional)).

### StreamPulse web portal (sibling `streampulse-web`)

From **streamclone** repo root (after laptop smoke passes):

```powershell
scripts\laptopworker-remote.cmd smoke
cd ..\streamclone-pulse\streampulse-web
$env:VITE_BACKEND_URL = 'http://laptopworker:8090'
npm run dev
```

Or from streamclone (if sibling checkout exists):

```bash
VITE_BACKEND_URL=http://laptopworker:8090 bash scripts/pulse-web-dev.sh
```

Portal reads `VITE_BACKEND_URL` at dev/build time (`streampulse-web/src/lib/apiClient.ts`). Default local dev remains `http://localhost:8090` (`streampulse-web/README.md`).

### Cursor / VS Code Remote SSH

Use when you want the editor on the laptop filesystem for long stack work:

1. Connect: `ssh aron@laptopworker` (Tailscale SSH)
2. Open folder: `~/streamclone`
3. Terminal on laptop for stack commands (`bash scripts/laptopworker-stack.sh …`)

Keep **Windows + multi-root workspace** as the default for cross-repo Pulse work (`streamclone-pulse-extension.code-workspace`). Remote SSH is optional for laptop-only ops.

**Recover unhealthy stack:**

```bash
bash scripts/laptopworker-stack.sh status
bash scripts/laptopworker-stack.sh logs
bash scripts/laptopworker-stack.sh smoke
systemctl --user status streamclone-dev.service
journalctl --user -u streamclone-dev.service -n 50 --no-pager
```

### Tailscale security checklist

| Check | Recommendation |
|-------|----------------|
| MagicDNS | Enabled — browse `http://laptopworker:8090` |
| Tailscale SSH | Enabled for `aron@laptopworker` |
| Key expiry | Disable for this trusted always-on node if admin policy allows |
| Exit node / subnet routes | **Off** on laptop unless explicitly approved |
| BearHost in `tailscale status` | Only when you need direct tailnet access to VPS services |
| `:8090` exposure | Run `bash scripts/laptopworker-stack.sh ufw-tailnet` once (DOCKER-USER + UFW) |
| Future ACL idea | Tag laptop as `tag:dev-hub`; allow Windows PC (+ BearHost if needed); deny broad tailnet access to `:8090` as the tailnet grows |

### Optional: VPS reverse proxy over Tailscale (future)

Document only — not implemented by default:

- Public users continue to hit **BearHost HTTPS** (`https://api.streampulse.stream`)
- BearHost *may* reverse-proxy selected routes to `http://laptopworker:PORT` over Tailscale for private demos
- Laptop must **not** receive home-router port forwards
- Never expose Postgres, Redis, or Minio on `0.0.0.0`

### Lightweight observability

```bash
systemctl --user list-timers | grep streamclone
docker system df
docker stats   # when debugging resource use
journalctl --user -u streamclone-dev-health.service -n 20 --no-pager
```

No Portainer or node exporter unless explicitly requested.

---

## Related

- BearHost production: `docs/bearhost-production.md`
- Azure hybrid (VPS scraper pattern): `docs/azure-archive-plane.md`
- Workspace layout: `docs/workspace.md`
