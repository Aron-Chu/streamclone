# Laptopworker dev hub

Tailnet host **`laptopworker`** runs the **core Streamclone dev stack** (UI, playback, chat, emotes, local Postgres). **Network-heavy work stays on legacy-rollback-host** (scraper, corpus workers, silver/gold backfill).

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

Commands on laptop: `bash scripts/laptopworker-stack.sh` → `start` | `stop` | `restart` | `status` | `logs` | `smoke` | `update` | `install-service` | `ufw-tailnet` | `enable-linger` | `boot-check`

From **Windows** (repo root), same stack via SSH:

| Command | Sudo at desk? |
|---------|----------------|
| `scripts\laptopworker-remote.cmd setup` | **Once** (then passwordless) |
| `scripts\laptopworker-remote.cmd setup-verify` | No |
| `scripts\laptopworker-remote.cmd smoke` | No |
| `scripts\laptopworker-remote.cmd boot-check` | No |
| `scripts\laptopworker-remote.cmd ufw-tailnet` | No (after `setup`) |

After `setup`, laptopworker scripts run via passwordless sudo — no need to visit the machine.

Browse UI on tailnet: **http://laptopworker:8090**

---

## Workload split

| Host | Runs | Does not run |
|------|------|--------------|
| **legacy-rollback-host** | Optional scraper / corpus workers (private ops) | — |
| **laptopworker** | Caddy `:8090`, frontend, metadata, video, chat, emote, postgres, redis, minio, mediamtx | Local scraper profile, corpus workers |
| **Windows PC** | Cursor, `laptopworker-remote.*` | — |

Env: `deploy/env/profile-laptopworker-dev.env` merged into `.env.local` (**user keys win**; bootstrap/update never clobber custom overrides).

Compose overlay: `deploy/docker-compose.laptopworker-dev.yml` (core-only stack)

Shared helpers: `scripts/laptopworker-env.sh`

---

## Operator checklist (Aron)

One-time on laptop (or from Windows — **one sudo password at your desk**):

```cmd
scripts\laptopworker-remote.cmd setup
```

This installs passwordless sudo for laptopworker scripts, enables linger, UFW+DOCKER-USER, always-on power, Ubuntu-first boot (GRUB+UEFI), and systemd. After that, everything below is remote with no walk-over.

Legacy manual steps (included in `setup`):

```bash
ssh aron@laptopworker
cd ~/streamclone
bash scripts/laptopworker-stack.sh setup
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

## Update flow (not automatic on git push)

| Product | Auto-deploy? | How it updates |
|---------|--------------|----------------|
| **laptopworker stack** | **No** — manual after `git push` to streamclone `master` | Windows: `scripts\laptopworker-remote.cmd update` (git pull + smart rebuild + smoke) |
| **legacy-rollback-host** | **No** — separate deploy scripts | Private **streampulse-ops** deploy scripts |
| **StreamPulse website** | **Yes** (when CI wired) | `streamclone-pulse/streampulse-web` → Cloudflare Pages on push |
| **Chrome extension** | **No** | Build `streamclone-pulse` → Load unpacked / store release |
| **Windows local stack** | **No** | `git pull` + `make up` / `make restart` yourself |

Boot on laptop **does** auto-start the stack (`streamclone-dev.service`) but does **not** auto `git pull`. After every streamclone push you intend to run on laptop:

```cmd
scripts\laptopworker-remote.cmd update
```

If firewall/sudo sbin changed, re-run `setup` once to refresh `/usr/local/sbin`.

Future (P3): optional webhook/auto-deploy on push — not implemented yet.

`scripts/laptopworker-update.sh`:

1. `git fetch` + fast-forward `master`
2. Merge `.env.local` (preserve overrides) + resynth `.env`
3. Detect changed paths between old/new SHA
4. Rebuild only affected services:

| Changed paths | Action |
|---------------|--------|
| `go.mod`, `go.sum` | Rebuild all Go services |
| `frontend/**` | Rebuild `frontend` |
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
| Production-like API | Use hosted StreamPulse API from **streamclone-pulse** / private ops — not the laptopworker `:8090` stack |
| VPS-only services | BearHost must appear in `tailscale status` only if laptop needs direct tailnet access to VPS |

---

## Security (tailnet-only)

Defense in depth (UFW alone does not filter Docker-published ports):

1. **Root-owned helpers** — `/usr/local/sbin/streamclone-laptopworker-*` (not the git checkout). Passwordless sudo grants **only** these paths.
2. **DOCKER-USER** — allow `:8090` on `lo` + `tailscale0`; drop other interfaces.
3. **INPUT** — same for host `docker-proxy` on `:8090` (blocks home-LAN IP access).
4. **Boot persistence** — `streamclone-laptopworker-firewall.service` reapplies rules after Docker/Tailscale start.
5. **UFW** — SSH on `tailscale0` only by default.

After changing `scripts/laptopworker/sbin/*`, refresh installed helpers:

```cmd
scripts\laptopworker-remote.cmd setup
scripts\laptopworker-remote.cmd ufw-tailnet
```

LAN block test from Windows (should **fail**):

```powershell
Invoke-WebRequest http://192.168.4.27:8090/ -TimeoutSec 5
```

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
curl -fsS http://127.0.0.1:8090/v1/metadata/health
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

## StreamPulse extension / portal (sibling repo)

Laptopworker runs the **core watch stack only** (`:8090`). StreamPulse extension and portal dev use **streamclone-pulse** with the hosted API by default — see sibling [`streamclone-pulse/docs/website-portal/local-dev-runbook.md`](../../streamclone-pulse/docs/website-portal/local-dev-runbook.md) and [`docs/streampulse-product-boundary.md`](streampulse-product-boundary.md).

### Cursor / VS Code Remote SSH

Use when you want the editor on the laptop filesystem for long stack work:

1. Connect: `ssh aron@laptopworker` (Tailscale SSH)
2. Open folder: `~/streamclone`
3. Terminal on laptop for stack commands (`bash scripts/laptopworker-stack.sh …`)

Keep **Windows + multi-root workspace** as the default for cross-repo work (`streamclone-pulse-extension.code-workspace`). Remote SSH is optional for laptop-only ops.

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

- Public StreamPulse users hit the **hosted API** (private ops) — not laptopworker `:8090`
- Legacy rollback host *may* reverse-proxy selected routes to `http://laptopworker:PORT` over Tailscale for private demos
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

- Hosting topology (laptop / PC): [`.kiro/steering/laptopworker-hosting.md`](../.kiro/steering/laptopworker-hosting.md)
- Product boundary: [streampulse-product-boundary.md](streampulse-product-boundary.md)
- Azure hybrid (VPS scraper pattern): `docs/azure-archive-plane.md`
- Workspace layout: `docs/workspace.md`
