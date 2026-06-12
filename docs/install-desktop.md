# Install Streamclone

**Need:** [Docker Desktop](https://docs.docker.com/desktop/) (running). **Not needed:** Git, Go, Node, Twitch login (for watching).

Open **`http://localhost:8090/`** when running. Use that URL only — not raw ports like `:8081`. There is no separate welcome page: first launch shows a dismissible **Stack status** overlay on the live directory with optional service start buttons.

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
| **Clip Studio** | `--profile clipper` or **Start Clip Studio** in Stack status | + ~1 GB `clipper` image | Optional **Sign in** on localhost (no Twitch CLI) | Clip Studio at `/studio` |
| **Full** | both profiles | Analytics + Clip Studio sizes combined | Scraper sibling; sign-in for clips | All optional features |

### Optional Twitch sign-in (chat send, follows, Clip Studio)

**Not required to watch.** When you want to send chat or use Clip Studio:

1. Open **`http://localhost:8090/`** (localhost only — not a tunnel URL).
2. Click **Sign in (optional)** in the header.
3. Approve on the Twitch tab — Streamclone signs you in automatically.

**You do not need Twitch CLI.** Official releases include a bundled Twitch OAuth app when the maintainer configures release secrets. Developers can copy `deploy/env/oauth-bundle.env.example` → `oauth-bundle.env` with their own [Twitch Developer](https://dev.twitch.tv/console/apps) app (no redirect URL required).

After sign-in, clip credentials sync to Clip Studio automatically when that tier is enabled.

If you only use Setup.exe, expect **Core Watch** behavior: Analytics pages show stream lists and session stats, but **per-minute charts stay empty** until you add the Analytics (scraper) tier. Use **Start Analytics** in the first-run overlay, the **Stack status** header button, or the banner on Analytics when the scraper is offline.

---

## Windows (recommended)

### First time (installer)

1. Start **Docker Desktop** (wait until Running)
2. From **[GitHub Releases](https://github.com/Aron-Chu/streamclone/releases/latest)**, download **`Streamclone-Setup-v*.exe`**
3. Run the installer — wizard extracts files to `%USERPROFILE%\streamclone`, sets up Docker, adds shortcuts, and **opens the directory**

Takes **~3–5 minutes** (pulls pre-built images; no local compile).

Windows may show **"Unknown Publisher"** — click **Run** or **More info → Run anyway**. Streamclone is open source but not code-signed yet (signing removes that warning).

**Antivirus / "virus detected" on Setup.exe:** Unsigned Inno Setup installers are often flagged as suspicious by heuristic scanners (Windows Defender, Norton, etc.) even when the file is safe. Streamclone is [open source](https://github.com/Aron-Chu/streamclone); releases are built by GitHub Actions. If your AV quarantines `Streamclone-Setup-*.exe`, use **`Install Streamclone.cmd`** instead (PowerShell + Docker only, no EXE), or add an exclusion for the downloaded installer. Code signing would reduce false positives but is not set up yet.

**Setup failed but the app works?** Re-running Install re-downloads the release bundle. If Docker containers from a previous install are still running, open **http://localhost:8090/** and run **`Check Streamclone.cmd`** in `%USERPROFILE%\streamclone` — it reports Docker, containers, and web UI status without changing anything.

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
| **Check Streamclone.cmd** | Diagnostic — Docker, containers, http://localhost:8090/ (no changes) | Yes | Yes |
| **Start Streamclone.cmd** | Start stack → open directory | Yes | Yes |
| **Stop Streamclone.cmd** | Stop containers (pause) | Yes | Yes |
| **Manage Streamclone.cmd** | Menu: status, repair (re-pull + restart), logs, uninstall | Yes | Repair keeps volumes |
| **Uninstall** (see below) | Remove everything — volumes, `.env`, shortcuts, install folder | **No** | **No** |

**Manage Streamclone** is the support console when something feels stuck: option **4 Repair** re-pulls GHCR images and recreates containers without wiping your database. First repair on a new PC can take several minutes (same as first install pull).

**Stop** = pause. Your `.env`, install folder, and database/MinIO volumes stay on disk. Run **Start** to resume.

**Uninstall** = complete removal. Three equivalent paths:

| How | Confirmation |
|-----|----------------|
| **Settings → Apps → Streamclone → Uninstall** | Windows uninstall wizard (recommended after Setup.exe install) |
| **Start menu → Streamclone → Uninstall Streamclone** | Same uninstall wizard |
| **`Uninstall Streamclone.cmd`** in the install folder | Type `YES` in the terminal |

All paths stop Docker, delete volumes and `.env`, remove Desktop shortcuts, and delete `%USERPROFILE%\streamclone`. Interactive uninstall asks whether to also remove downloaded Streamclone Docker images. Keep images for faster reinstall/repair; remove images to reclaim disk space or simulate a first-time install. Advanced non-interactive image cleanup: `powershell -File scripts\uninstall-streamclone.ps1 -PruneImages`.

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

## First open and optional services

Install and **Start** open **`http://localhost:8090/`** (the live directory). There is no separate welcome page.

- **First visit:** a blurred overlay welcomes you with **Browse live streams** (primary action) and optional **Analytics (scraper)** / **Clip Studio (clipper)** status cards.
- **Stack status:** header button on the directory, Analytics, and Clip Studio reopens that panel anytime.
- **Analytics without scraper:** open a channel → Analytics — use **Start Analytics** in the banner or Stack status (requires **Start Streamclone** so setup-control is running).
- **Clip Studio without clipper:** same pattern with **Start Clip Studio** on the error screen or Stack status.
- Legacy `/welcome` URLs redirect to `/` and show the overlay once.

Core Watch does not require scraper or clipper. Minute-level charts and Clip Studio are optional tiers — see [options.md](options.md).

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
