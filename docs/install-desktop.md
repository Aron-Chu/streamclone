# Install Streamclone

**Need:** [Docker Desktop](https://docs.docker.com/desktop/) (running). **Not needed:** Git, Go, Node, Twitch login (for watching).

Open **`http://localhost:8090/welcome`** when running. Use that URL only — not raw ports like `:8081`.

---

## Windows (recommended)

### First time (one double-click)

1. Start **Docker Desktop** (wait until Running)
2. From **[GitHub Releases](https://github.com/Aron-Chu/streamclone/releases/latest)**, download **`Install Streamclone.cmd`** only
3. Double-click it — downloads the release, sets up Docker, adds Desktop shortcuts, and **opens the welcome page**

Windows may show **"Unknown Publisher"** — click **Run**. Streamclone is not code-signed yet (no `.exe` installer).

Takes **~3–5 minutes** (pulls pre-built images; no local compile).

**Alternative:** download `streamclone-*-windows.zip`, extract, then run **`Install Streamclone.cmd`** inside the folder (same result; you unzip manually).

### Lifecycle

| Launcher | What it does | Keeps install folder? | Keeps data volumes? |
|----------|----------------|----------------------|---------------------|
| **Install Streamclone.cmd** | First-time setup — config, pull images, shortcuts, open welcome | Yes | Creates fresh volumes |
| **Start Streamclone.cmd** | Start stack → open welcome page | Yes | Yes |
| **Stop Streamclone.cmd** | Stop containers (pause) | Yes | Yes |
| **Uninstall Streamclone.cmd** | Remove everything — volumes, `.env`, shortcuts, install folder | **No** | **No** |

**Stop** = pause. Your `.env`, install folder, and database/MinIO volumes stay on disk. Run **Start** to resume.

**Uninstall** = complete removal. Type `YES` to confirm. Deletes Docker volumes (all local stream data), secrets in `.env`, Desktop shortcuts, and the install folder. Optional advanced flag: `powershell -File scripts\uninstall-streamclone.ps1 -PruneImages` also removes downloaded `ghcr.io/aron-chu/streamclone/*` images.

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
| **Install** | First-time setup |
| **Start** | Daily open |
| **Stop** | Shutdown containers only |
| **Uninstall** | Complete teardown |

Nothing is uploaded to GitHub. Images come from **`ghcr.io/aron-chu/streamclone/*`** (published on each `v*` release).

---

## Releases vs Packages

| | **Releases** | **Packages** (GHCR) |
|---|---|---|
| **What** | ZIP + launchers (`Install`, `Start`, `Stop`, `Uninstall`) | Pre-built Docker images |
| **For you** | Download and double-click | Pulled automatically by Install |
| **URL** | [releases/latest](https://github.com/Aron-Chu/streamclone/releases/latest) | `ghcr.io/aron-chu/streamclone/metadata`, `chat`, `video`, etc. |

**Maintainer:** tag `v*` (e.g. `v0.1.1`) → CI publishes both. GHCR packages must be **public** for installs without `docker login`.

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
| Git-clone build too slow | Use the **release ZIP** or `setup.ps1 -UseImages` |
| Frontend fails on release ZIP | Fixed in v0.1.1+ (`docker-compose.release.yml` — no host nginx mount) |
| Start from scratch | Run **Uninstall**, then **Install** again |
