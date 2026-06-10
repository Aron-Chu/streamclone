# Getting started with Streamclone

Streamclone is a self-hosted Twitch-style directory with HLS playback, emote-rich chat, analytics, and optional Clip Studio. Local dev runs entirely in Docker behind **`http://localhost:8090`**.

You do **not** need to copy `.env.example` for local setup. Use the setup wizard instead.

---

## Prerequisites

| Required | What | Notes |
|----------|------|--------|
| **Docker** | Container runtime + Compose v2 | **Necessary today** — the stack is Postgres, Redis, MinIO, MediaMTX, Go services, FFmpeg workers, and a proxy. There is no supported native “install without Docker” path. |
| **Git** | Clone / updates | Installers assume a git checkout |

**Docker Desktop vs Engine**

- **Windows / macOS:** Docker Desktop is the practical choice (includes Compose, GUI, WSL2 integration on Windows).
- **Linux:** Docker **Engine** + `docker compose` plugin is enough — Desktop is optional.
- Alternatives (Rancher Desktop, Podman with compose) may work but are not tested in CI.

**Not required for core viewing:** Go, Node, Python, Postgres, Redis installed on the host — those run inside containers.

Optional:

- [Twitch CLI](https://github.com/twitchdev/twitch-cli) — OAuth app creds + device-code login for chat write / Clip Studio
- Sibling repo [streamclone-scraper](https://github.com/Aron-Chu/streamclone-scraper) — TwitchTracker viewer charts (`scraper` / `full` profiles)

**Windows:** see [`.kiro/steering/windows-dev.md`](../.kiro/steering/windows-dev.md) for `wslrelay` / stale localhost issues.

---

## Non-developer quick path

Check dependencies, then start (first run runs setup automatically):

```powershell
# Windows
powershell -File scripts/preflight-deps.ps1 -InstallHints
powershell -File scripts/start-streamclone.ps1
```

```sh
# Linux / macOS
bash scripts/preflight-deps.sh --install-hints
bash scripts/start-streamclone.sh
```

| Script | Purpose |
|--------|---------|
| `preflight-deps.*` | Docker running? Port 8090 free? Git installed? |
| `start-streamclone.*` | Setup on first run, start stack, open browser |
| `stop-streamclone.*` | Stop all compose profiles |
| `install.ps1` / `install.sh` | Clone repo + full setup wizard |

**Profiles for non-devs:** stick with **core** (default) — watch streams and chat without Twitch login. Choose **clipper** or **full** only if you need Clip Studio or analytics charts (extra manual steps).

Faster repeat installs: `start-streamclone.ps1 -UseImages` pulls pre-built GHCR images instead of building locally (~15 min saved on first run).

---

## Three ways to install

| Tier | Command | Best for |
|------|---------|----------|
| **1. Git clone** | `make setup` | Contributors, fastest iteration (`--build`) |
| **2. Pre-built images** | `make setup SETUP_MODE=release` or `scripts/setup.sh --use-images` | Skip local image builds; pull from GHCR |
| **3. One-click** | `curl -fsSL …/scripts/install.sh \| bash` | Fresh machine with Docker only |

### Tier 1 — Interactive setup (recommended)

```sh
git clone https://github.com/Aron-Chu/streamclone.git
cd streamclone
make setup
```

**Windows (no make):**

```powershell
git clone https://github.com/Aron-Chu/streamclone.git
cd streamclone
powershell -ExecutionPolicy Bypass -File scripts/setup.ps1
```

The wizard will:

1. Check Docker
2. Ask for a **profile** (see below)
3. Synthesize `.env` from `.env.dev` + profile fragment + generated secrets
4. Optionally sync Twitch CLI OAuth creds
5. Optionally clone `streamclone-scraper` next to this repo
6. Start compose and run smoke checks

Non-interactive (CI / scripts):

```sh
scripts/setup.sh --profile core --non-interactive
scripts/setup.sh --profile full --non-interactive --skip-twitch
```

### Tier 2 — Pre-built Docker images

After a `v*` release tag, images are published to `ghcr.io/aron-chu/streamclone/<service>`.

```sh
make setup SETUP_MODE=release
# or
IMAGE_TAG=v1.2.3 scripts/setup.sh --profile core --use-images --non-interactive
```

You still need this repo (or a release bundle) for compose files, Caddy, and migrations — images alone are not enough.

### Tier 3 — Installer script

```sh
curl -fsSL https://raw.githubusercontent.com/Aron-Chu/streamclone/main/scripts/install.sh | bash
```

Options: `--profile full`, `--dir ~/streamclone`, `--use-images`, `--non-interactive`.

---

## Profiles

| Profile | Compose | What you get |
|---------|---------|----------------|
| `core` | default services | Directory, playback, chat, emotes, analytics — **anonymous viewing works** |
| `scraper` | `--profile scraper` | Core + TwitchTracker charts (needs sibling scraper repo) |
| `clipper` | `--profile clipper` | Core + live clipper / Clip Studio |
| `full` | both profiles | Scraper + clipper |

Makefile shortcuts:

```sh
make setup-core    # core, non-interactive
make setup-full    # full, non-interactive
make up-scraper    # start scraper profile + Camoufox TwitchTracker preflight
make up-full       # scraper + clipper (+ preflight)
make scraper-check # probe only (no auto-fix)
make scraper-preflight  # probe + auto-clear locks / recreate scraper
make scraper-warm  # one-time headful Camoufox CF warmup (Windows)
make scraper-reload    # force-recreate scraper after .env changes

Set `SCRAPER_SKIP_PREFLIGHT=1` to skip the post-start scrape probe on `make up-scraper`.
```

---

## Environment files

| File | Purpose |
|------|---------|
| `.env.dev` | Minimal committed defaults — **do not edit for secrets** |
| `deploy/env/profile-*.env` | Per-profile overrides merged by setup |
| `.env` | Generated local env (gitignored) |
| `.env.local` | Optional overrides (gitignored) merged last |
| `.env.example` | Advanced reference only — not required for setup |

Validate your `.env`:

```sh
make validate-env
scripts/validate-env.sh --profile full
scripts/validate-env.sh --fix   # regenerate placeholder secrets
```

---

## Twitch authentication

**Core viewing** does not require Twitch login.

For chat write, follows, or Clip Studio:

1. Install Twitch CLI and run `twitch configure` (one-time Developer app)
2. `make twitch-sync` — copies client id/secret into `.env` and recreates `chat`, `metadata`, `analytics`, `emote` (`make reload-env` after any manual `.env` edit)
3. `make twitch-local-auth` — device-code flow; syncs clipper access + refresh tokens into `.env` and recreates clipper

**When tokens expire** (clipper “expired or revoked”, stale emote OAuth in containers):

```sh
make refresh-auth          # auto-refresh clipper token when refresh token exists; reload stale services
make twitch-local-auth     # full re-login when refresh fails (approve Twitch in browser)
make rebuild               # stop + up-full + refresh-auth (clean slate after .env/auth changes)
```

`docker compose restart` does **not** reload `.env` — use `make reload-env`, `make ensure-clipper-auth`, or `make rebuild`.

`TWITCH_DEV_TOKEN_IMPORT_ENABLED=true` (set by setup) enables in-app **Use local token** in the UI.

---

## Scraper sibling layout

```
parent/
  streamclone/          ← this repo
  streamclone-scraper/  ← optional, for analytics charts
```

Setup can clone the scraper for you. For Cloudflare-blocked TwitchTracker egress, add `PROXY_*` vars to `.env.local` (never commit).

---

## Troubleshooting

### Smoke checks fail on Windows

- Use **`http://localhost:8090`** only (Caddy proxy), not raw service ports
- Stale data / connection refused: `wsl --shutdown`, then `make down` and `make setup`

### Clipper / scraper health timeouts

- Clipper: run `make refresh-auth` first; if still failing, `make twitch-local-auth` (approve Twitch login)
- Scraper: confirm sibling repo exists and `docker ps` shows `streamclone-scraper`

### Re-run setup safely

```sh
scripts/setup.sh --profile core --non-interactive --no-up   # refresh .env only
make down && make setup
```

---

## Next steps

- `make smoke` — core health checks
- `make smoke-ui` — adds Playwright UI smoke
- [CONTRIBUTING.md](../CONTRIBUTING.md) — tests before PR
- [`.kiro/steering/`](../.kiro/steering/) — agent/developer deep docs
