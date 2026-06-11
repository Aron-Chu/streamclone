# Install Streamclone

**Need:** [Docker Desktop](https://docs.docker.com/desktop/) (running). **Not needed:** Git, Go, Node, Twitch login (for watching).

Open **`http://localhost:8090/`** when running. Use that URL only — not raw ports like `:8081`.

### System requirements

| | Minimum | Recommended |
|---|---------|-------------|
| **OS** | Windows 10/11 64-bit, macOS 12+, Linux with Docker | Same |
| **RAM** | 8 GB (Docker + Streamclone stack) | 16 GB |
| **Disk** | 5 GB free (images + volumes) | 10 GB+ |
| **CPU** | 4 cores | 6+ cores |
| **Network** | Stable broadband; **~1.5–2 GB** first-time download | Wired or fast Wi‑Fi |

**First install time:** ~3–8 minutes depending on network (image pull dominates). **Daily Start:** ~10–30 seconds if images are cached.

**`Streamclone-Setup.exe`** shows **in-wizard progress** during setup (pulling images, starting containers, health checks) — no extra terminal window.

### What downloads on first install

**GHCR pull sizes measured at `v0.1.4-rc1`** (2026-06-11; re-run on final `v0.1.4` tag after release). Local image size after `docker pull` — compressed registry download may differ slightly.

| Image | `v0.1.4-rc1` (MB) | Notes |
|-------|-------------------|-------|
| `video` | 232.2 | Largest core service (trimmed from ~900 MB pre-Alpine) |
| `emote` | 107.5 | Emote proxy (trimmed from ~430 MB) |
| `frontend` | 20.3 | Static UI + nginx |
| `metadata` | 7.1 | |
| `chat` | 7.1 | |
| `analytics` | 8.2 | |
| Third-party (`postgres`, `minio`, `caddy`, …) | varies | Not published to `ghcr.io/aron-chu/streamclone/*` |
| **Total (6 core GHCR images)** | **382.5 MB** | Plus third-party layers on first `docker compose up` |

Reinstalls and **Start** use `--pull missing` so already-downloaded images are not re-fetched.

### Product tiers

**`Streamclone-Setup.exe` installs Core Watch only.** Optional tiers are enabled later with compose profiles (see [scraper-cloudflare-and-proxy.md](./scraper-cloudflare-and-proxy.md) for Analytics).

| Tier | How you get it | First-pull download | Prerequisites | Without scraper |
|------|----------------|---------------------|---------------|-----------------|
| **Core Watch** | Setup.exe / default install | **~383 MB** core GHCR images at `v0.1.4-rc1` (+ third-party on first compose up) | Docker Desktop running | Directory, live playback, chat, emotes, Helix/VOD stream history, TwitchTracker **summary** stats (avg/peak on stream rows) |
| **Analytics** | `setup.ps1 -Profile scraper` or compose `--profile scraper` | + scraper image (builds from sibling repo; **not** published to GHCR) | Clone [`streamclone-scraper`](https://github.com/Aron-Chu/streamclone-scraper) beside this repo | Minute-level viewer charts on Analytics, reliable TwitchTracker sync |
| **Clip Studio** | `--profile clipper` | + ~1 GB `clipper` image | Twitch CLI + device login (`make twitch-local-auth`) | Clip Studio at `/studio` |
| **Full** | both profiles | Analytics + Clip Studio sizes combined | Scraper sibling + Twitch CLI | All optional features |

If you only use Setup.exe, expect **Core Watch** behavior: Analytics pages show stream lists and session stats, but **per-minute charts stay empty** until you add the Analytics (scraper) tier.

---

## Windows (recommended)

### First time (installer)

1. Start **Docker Desktop** (wait until Running)
2. From **[GitHub Releases](https://github.com/Aron-Chu/streamclone/releases/latest)**, download **`Streamclone-Setup-v*.exe`**
3. Run the installer — wizard extracts files to `%USERPROFILE%\streamclone`, sets up Docker, adds shortcuts, and **opens the directory**

Takes **~3–5 minutes** (pulls pre-built images; no local compile).

Windows may show **"Unknown Publisher"** — click **Run** or **More info → Run anyway**. Streamclone is open source but not code-signed yet (signing removes that warning).

**Alternatives**

| Method | When to use |
|--------|-------------|
| **`Install Streamclone.cmd`** | One-file download; fetches the release ZIP automatically |
| **`streamclone-*-windows.zip`** | Manual extract, then run **`Install Streamclone.cmd`** inside |

### Lifecycle

| Launcher | What it does | Keeps install folder? | Keeps data volumes? |
|----------|----------------|----------------------|---------------------|
| **Streamclone-Setup.exe** | First-time setup — wizard, config, pull images, shortcuts, open directory | Yes | Creates fresh volumes |
| **Install Streamclone.cmd** | Same as Setup.exe (without wizard UI) | Yes | Creates fresh volumes |
| **Start Streamclone.cmd** | Start stack → open directory | Yes | Yes |
| **Stop Streamclone.cmd** | Stop containers (pause) | Yes | Yes |
| **Uninstall** (see below) | Remove everything — volumes, `.env`, shortcuts, install folder | **No** | **No** |

**Stop** = pause. Your `.env`, install folder, and database/MinIO volumes stay on disk. Run **Start** to resume.

**Uninstall** = complete removal. Three equivalent paths:

| How | Confirmation |
|-----|----------------|
| **Settings → Apps → Streamclone → Uninstall** | Windows uninstall wizard (recommended after Setup.exe install) |
| **Start menu → Streamclone → Uninstall Streamclone** | Same uninstall wizard |
| **`Uninstall Streamclone.cmd`** in the install folder | Type `YES` in the terminal |

All paths stop Docker, delete volumes and `.env`, remove Desktop shortcuts, and delete `%USERPROFILE%\streamclone`. The **Setup.exe uninstall wizard** shows in-app progress (no extra CMD window). Docker images stay cached for faster reinstall; optional advanced: `powershell -File scripts\uninstall-streamclone.ps1 -PruneImages`.

---

## macOS

1. Download **`streamclone-*.tar.gz`** from [Releases](https://github.com/Aron-Chu/streamclone/releases/latest) → extract to `~/streamclone`
2. Double-click **`launchers/Install Streamclone.command`**
3. Daily: **Streamclone Start** / **Stop** in `~/Applications`
4. Remove everything: **`launchers/Uninstall Streamclone.command`**

---

## What each launcher does

| File | Role |
|------|------|
| **Setup.exe / Install** | First-time setup |
| **Start** | Daily open |
| **Stop** | Shutdown containers only |
| **Uninstall** | Complete teardown |

Nothing is uploaded to GitHub. Images come from **`ghcr.io/aron-chu/streamclone/*`** (published on each `v*` release).

---

## Releases vs Packages

| | **Releases** | **Packages** (GHCR) |
|---|---|---|
| **What** | Setup.exe, ZIP + launchers (`Install`, `Start`, `Stop`, `Uninstall`) | Pre-built Docker images |
| **For you** | Download and run | Pulled automatically by Install |
| **URL** | [releases/latest](https://github.com/Aron-Chu/streamclone/releases/latest) | `ghcr.io/aron-chu/streamclone/metadata`, `chat`, `video`, etc. |

**Maintainer:** tag `v*` (e.g. `v0.1.4`) → CI publishes images, ZIP, `.cmd`, and **Setup.exe**. GHCR packages must be **public** for installs without `docker login`.

---

## Slower path: git clone (developers)

Builds images locally — **~10–20 minutes** first run.

```powershell
git clone https://github.com/Aron-Chu/streamclone.git
cd streamclone
powershell -File scripts\setup.ps1
```

Faster clone install: pull release images instead of building:

```powershell
powershell -File scripts\setup.ps1 -UseImages
```

See [CONTRIBUTING.md](../CONTRIBUTING.md) for tests and PR workflow. Optional features: [options.md](options.md).

---

## Troubleshooting

| Problem | Fix |
|---------|-----|
| Docker not running | Start Docker Desktop, retry |
| Port 8090 in use | Run **Stop Streamclone**, or free the port |
| Images fail to pull | Set GHCR packages to **Public**, or run `docker login ghcr.io` |
| Git-clone build too slow | Use **Setup.exe** or the **release ZIP** |
| Frontend fails on release ZIP | Fixed in v0.1.1+ (`docker-compose.release.yml` — no host nginx mount) |
| Start from scratch | Run **Uninstall**, then **Setup.exe** or **Install** again |
| Install feels slow | Normal on first run (large `video`/`emote` images). Use wired network; second Start is much faster |
| Setup.exe stuck on "Pulling Docker images" | Normal on first install (~1.5 GB). Ensure Docker Desktop is running and network is stable |
