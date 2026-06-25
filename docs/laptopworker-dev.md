# Laptopworker dev hub

Tailnet host **`laptopworker`** runs the **core Streamclone dev stack** (UI, playback, chat, emotes, local Postgres). **Network-heavy work stays on BearHost VPS** (scraper, corpus workers, silver/gold backfill).

## Quick control (Windows)

From the **streamclone repo root** (not `C:\Windows\System32`):

```powershell
powershell -ExecutionPolicy Bypass -File scripts\laptopworker-remote.ps1 status
```

Or:

```cmd
scripts\laptopworker-remote.cmd status
```

Commands: `start` | `stop` | `restart` | `status` | `smoke` | `logs` | `update` | `install-service`

Browse UI on tailnet: **http://laptopworker:8090**

---

## Workload split

| Host | Runs | Does not run |
|------|------|--------------|
| **BearHost VPS** | `scraper`, `analytics-workers`, corpus/silver/gold, TwitchTracker/Camoufox | — |
| **laptopworker** | Caddy `:8090`, frontend, metadata, video, chat, emote, analytics API, postgres, redis, minio, mediamtx | Local scraper profile, storygraph ingest, corpus workers |
| **Windows PC** | Cursor, `laptopworker-remote.*` | — |

Env: `deploy/env/profile-laptopworker-dev.env` merged into `.env.local` (user keys win; bootstrap/update do **not** clobber custom overrides).

Compose overlay: `deploy/docker-compose.laptopworker-dev.yml` (disables `storygraph`)

Shared helpers: `scripts/laptopworker-env.sh`

---

## What the laptop must host (core stack)

### Docker services (always when stack is up)

| Service | Purpose | Host port |
|---------|---------|-----------|
| `local-proxy` (Caddy) | Single entry `:8090` | `0.0.0.0:8090` |
| `frontend` | React UI | internal |
| `metadata` | Directory, Helix, VOD lists | `127.0.0.1:8081` |
| `video` | HLS relay, streamlink | `127.0.0.1:8082` |
| `chat` | IRC bridge, Twitch OAuth | `127.0.0.1:8083` |
| `emote` | 7TV/FFZ pipeline | `127.0.0.1:8084` |
| `analytics` | Pulse BFF, extension routes | `127.0.0.1:8086` |
| `postgres` | Local dev DB | `127.0.0.1:5432` |
| `redis` | Cache, chat buffer | `127.0.0.1:6379` |
| `minio` | Emote object storage | `127.0.0.1:9000` |
| `mediamtx` | RTMP/HLS origin | `127.0.0.1:8888` |
| `migrate` | One-shot schema apply | — |

### Host packages

| Package | Required | Notes |
|---------|----------|-------|
| Docker Engine + compose plugin | Yes | `get.docker.com`; user in `docker` group |
| git, curl, make, jq | Yes | Bootstrap installs |
| Tailscale | Yes | MagicDNS `laptopworker` |
| Go / Node on host | No | Services run in containers |

### Disk / RAM (observed)

| Resource | Minimum | This host |
|----------|---------|-----------|
| RAM | 8 GB | 15 GB |
| Disk | ~20 GB free for images + PG | ~87 GB free |
| Build cache | prune periodically | `docker builder prune` |

### Secrets (`~/.streamclone/secrets/`)

Copy from dev PC when needed (never commit):

| Secret | Used for |
|--------|----------|
| `deploy/env/oauth-bundle.env` | Twitch OAuth app (Sign in, Helix) |
| Twitch user tokens | Imported via dev token import at `http://laptopworker:8090` |
| Azure archive connection string | Only if enabling archive export locally (default **off**) |

Optional VPS reference (read-only): `STREAMPULSE_PUBLIC_API=https://api.streampulse.stream`

### Network (15 Mbps home)

- **Playback** uses bandwidth when you watch streams through the laptop stack.
- **Scrape / corpus / Pulse Wire ingest** must stay on VPS — local profile sets `STREAMCLONE_DISABLE_LOCAL_SCRAPER=true`, `PULSE_WIRE_ENABLED=false`, storygraph overlay off.
- Tailscale access to `:8090` is light (UI/API only).

### Power (always-on worker)

Applied by `scripts/laptopworker-power-config.sh`:

- Lid closed → **ignore** (no suspend)
- Idle → **ignore**
- Sleep targets → **masked**
- Keep **AC connected**

---

## Always-on after push

1. **One-time on laptop** (after bootstrap):

   ```bash
   cd ~/streamclone
   bash scripts/laptopworker-install-service.sh
   ```

   Installs `streamclone-dev.service` (start on boot) + health timer (smoke every 10 min). Requires `sudo` once for `loginctl enable-linger`.

2. **After merging to `master`**, from Windows repo root:

   ```powershell
   make laptopworker-update
   ```

   Requires laptopworker files on `origin/master`. Merges `.env.local` (preserves user overrides), then `compose up -d`.

3. **Container restart policy**: compose services use `restart: unless-stopped` — Docker daemon restarts unhealthy containers; full stack comes back on boot via systemd user unit.

4. **Boot without login** — run once on laptop if `loginctl show-user aron -p Linger` shows `no`:

   ```bash
   sudo loginctl enable-linger aron
   ```

There is **no auto-pull on every push** (by design — avoids surprise deploys). Run `update` after you push, or add a cron/timer that runs `laptopworker-update.sh` if you want hands-off sync.

### Security note

Caddy binds `:8090` on all interfaces; restrict with host UFW to `tailscale0` if you need tailnet-only exposure (not automated in bootstrap yet).

---

## Bootstrap (fresh laptop)

```bash
ssh aron@laptopworker
cd ~/streamclone   # or clone first
bash scripts/laptopworker-bootstrap.sh
bash scripts/laptopworker-install-service.sh
sudo loginctl enable-linger aron
```

---

## Health checks

```bash
bash scripts/laptopworker-stack.sh smoke
curl -fsS http://127.0.0.1:8090/v1/extension/health
```

---

## Related

- BearHost production: `docs/bearhost-production.md`
- Azure hybrid (VPS scraper pattern): `docs/azure-archive-plane.md`
- Local env synthesis: `scripts/setup.sh`, `scripts/lib/env.sh`
