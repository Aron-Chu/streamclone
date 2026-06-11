# Install Streamclone

**Need:** [Docker Desktop](https://docs.docker.com/desktop/) (running). **Not needed:** Git, Go, Node, Twitch login (for watching).

Open **`http://localhost:8090`** when running. Use that URL only — not raw ports like `:8081`.

---

## Windows (recommended)

### First time (one double-click)

1. Start **Docker Desktop** (wait until Running)
2. From **[GitHub Releases](https://github.com/Aron-Chu/streamclone/releases/latest)**, download **`Install Streamclone.cmd`** only
3. Double-click it — it downloads the release, sets up Docker, adds Desktop shortcuts, and **opens the welcome page**

Takes **~3–5 minutes** (pulls pre-built images; no local compile).

**Alternative:** download `streamclone-*-windows.zip`, extract, then run **`Install Streamclone.cmd`** inside the folder (same result; you unzip manually).

### Daily use

| Launcher | What it does |
|----------|----------------|
| **Start Streamclone** | Starts Docker stack → opens **`http://localhost:8090/welcome`** |
| **Stop Streamclone** | Stops all Streamclone containers |

---

## macOS

1. Download **`streamclone-*.tar.gz`** from [Releases](https://github.com/Aron-Chu/streamclone/releases/latest) → extract to `~/streamclone`
2. Double-click **`launchers/Install Streamclone.command`**
3. Daily: **Streamclone Start** / **Stop** in `~/Applications`

---

## What each launcher does

| File | Role |
|------|------|
| **Install Streamclone.cmd** | **First-time setup** — download/extract (if needed), config, pull images, start stack, Desktop shortcuts, open welcome page |
| **Start Streamclone.cmd** | **Daily open** — ensure stack is running, open welcome page |
| **Stop Streamclone.cmd** | **Shutdown** — stop all containers |

Nothing is uploaded to GitHub. Images come from **`ghcr.io/aron-chu/streamclone/*`** (published on each `v*` release).

---

## Releases vs Packages

| | **Releases** | **Packages** (GHCR) |
|---|---|---|
| **What** | ZIP + `Install Streamclone.cmd` | Pre-built Docker images |
| **For you** | Download and double-click | Pulled automatically by Install |
| **URL** | [releases/latest](https://github.com/Aron-Chu/streamclone/releases/latest) | `ghcr.io/aron-chu/streamclone/metadata`, `chat`, `video`, etc. |

**Maintainer:** tag `v*` (e.g. `v0.1.0`) → CI publishes both. GHCR packages must be **public** for installs without `docker login`.

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
