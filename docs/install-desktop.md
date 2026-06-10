# Install Streamclone on your desktop

Streamclone runs locally in Docker. You do not need Git, Go, or Node — only Docker Desktop and a few double-clicks.

## Prerequisites

1. Install [Docker Desktop](https://docs.docker.com/desktop/) (Windows or macOS).
2. Start Docker Desktop and wait until it reports **Running**.

On Windows, if `winget` is available:

```powershell
winget install Docker.DockerDesktop
```

---

## Windows — click only (recommended)

### First-time install

1. Download **`Install Streamclone.cmd`** from the [latest GitHub Release](https://github.com/Aron-Chu/streamclone/releases/latest)  
   *(Or from this repo: open the repo on GitHub → **Code** → **Download ZIP**, extract, double-click **`Install Streamclone.cmd`** in the folder.)*
2. Double-click **`Install Streamclone.cmd`**.
3. Wait for the installer to finish (first run pulls Docker images; may take a few minutes).
4. When done, you will have **Start Streamclone** and **Stop Streamclone** shortcuts on your Desktop.

### Every day after that

| Action | What to do |
|--------|------------|
| **Start** | Double-click **Start Streamclone** on your Desktop |
| **Use app** | Browser opens **http://localhost:8090** automatically when ready |
| **Stop** | Double-click **Stop Streamclone** on your Desktop |

No PowerShell, no terminal commands required.

---

## macOS — click only

### First-time install

1. Download the release **`.tar.gz`** from [GitHub Releases](https://github.com/Aron-Chu/streamclone/releases/latest) and extract it to `~/streamclone`,  
   **or** clone/download this repo.
2. Open the **`launchers`** folder.
3. Double-click **`Install Streamclone.command`** (macOS may ask you to allow it the first time).
4. Shortcuts appear in **`~/Applications`** as **Streamclone Start**, **Streamclone Stop**, and **Streamclone Install**.

### Every day after that

| Action | What to do |
|--------|------------|
| **Start** | Double-click **Streamclone Start** in `~/Applications` |
| **Use app** | Open **http://localhost:8090** in your browser |
| **Stop** | Double-click **Streamclone Stop** |

---

## Advanced: command-line install

If you prefer a one-liner:

**Windows:**

```powershell
powershell -ExecutionPolicy Bypass -File scripts/install.ps1 -Release -NonInteractive
```

**macOS / Linux:**

```bash
bash scripts/install.sh --release --non-interactive --use-images
```

---

## Optional: Clip Studio

Core install does **not** require Twitch login. For clipping (`/studio`), see [getting-started.md](getting-started.md).

---

## Troubleshooting

**Docker not running** — start Docker Desktop; double-click **Start Streamclone** again.

**Port 8090 in use** — stop other stacks or change the proxy port in `.env` (advanced).

**Images fail to pull** — GHCR packages must be **public** or you need `docker login ghcr.io`.

**Install fails before any release exists** — use the git-clone path in [README.md](../README.md) until the first `v*` release is published.

More detail: [getting-started.md](getting-started.md)

## Developers

Contributors should use git clone + `make setup` — see [README.md](../README.md).
