# Streamclone — Install & Infrastructure Review Brief

**Purpose of this document:** Provide a self-contained briefing for an external reviewer (human or large model) to **evaluate, benchmark, and critique** Streamclone’s install experience, deployment architecture, and infrastructure decisions. The reviewer should treat this as the primary context; follow file paths into the repo for implementation detail.

**Repository:** [github.com/Aron-Chu/streamclone](https://github.com/Aron-Chu/streamclone)
**Canonical local URL:** `http://localhost:8090/` (never raw service ports for end users)
**License:** Apache 2.0 (open source, not code-signed on Windows yet)

---

## 1. What Streamclone Is

Streamclone is a **self-hosted Twitch-style directory** with:

- Live HLS playback (Streamlink → FFmpeg → MediaMTX relay)
- Real-time chat (anonymous read; optional Twitch OAuth for send)
- 7TV emote rendering and CDN
- Channel analytics (VOD sync, charts)
- Optional Clip Studio (vertical clips + ASR) and TwitchTracker scraper (separate repo)

It is **not** a static site. It requires long-running processes: Go microservices, Redis, PostgreSQL, MinIO, MediaMTX, FFmpeg, and optionally Python clipper/scraper workers.

**Design north star:** A non-technical user with **Docker Desktop only** can install from a release EXE/ZIP, open the directory in a browser, and watch streams **without Twitch login**. Login is optional (chat badges, follows, Clip Studio).

---

## 2. Architecture Overview

### 2.1 High-level data flow

```mermaid
flowchart TB
  subgraph browser [Browser]
    UI[React SPA]
  end

  subgraph proxy [Caddy local-proxy :8090]
    Caddy[Caddy reverse proxy]
  end

  subgraph go [Go services]
    metadata[metadata :8080]
    video[video :8080]
    chat[chat :8080]
    emote[emote :8080]
    analytics[analytics :8080]
  end

  subgraph data [Data layer]
    redis[(Redis)]
    postgres[(PostgreSQL)]
    minio[(MinIO S3)]
  end

  subgraph media [Media pipeline]
    streamlink[Streamlink subprocess]
    ffmpeg[FFmpeg subprocess]
    mediamtx[MediaMTX HLS :8888]
  end

  UI --> Caddy
  Caddy -->|"/" SPA| frontend[frontend :80]
  Caddy -->|"/v1/*" APIs| metadata
  Caddy -->|"/v1/stream"| video
  Caddy -->|"/v1/auth*, /v1/ws"| chat
  Caddy -->|"/v1/emotes*, /emotes/*"| emote
  Caddy -->|"/v1/analytics/*"| analytics
  Caddy -->|"/live/*"| mediamtx

  metadata --> redis
  metadata --> postgres
  chat --> redis
  emote --> postgres
  emote --> minio
  analytics --> postgres
  video --> streamlink
  streamlink --> ffmpeg
  ffmpeg --> mediamtx
```

### 2.2 Service boundaries (runtime)

| Runtime | Services | Heavy work delegated to |
|---------|----------|-------------------------|
| Go (`cmd/*`) | metadata, video, chat, analytics, emote | Streamlink, FFmpeg, MediaMTX, `vips` CLI |
| Python (`clipper/`) | clipper (optional profile) | FFmpeg, Streamlink, faster-whisper |
| Python (sibling repo) | scraper (optional profile) | Camoufox/Chromium CDP |
| React + nginx | frontend | hls.js in browser |

Go services are **I/O-bound glue** around upstream Twitch and subprocess media tools. The scraper lives in a **separate repository** (`streamclone-scraper`) to isolate browser automation.

### 2.3 Compose file layering

All install/start paths assemble the same stack pattern:

```bash
docker compose --env-file .env \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.local-tunnel.yml \
  [-f deploy/docker-compose.release.yml]   # GHCR images, no local build
  [--profile scraper] [--profile clipper] \
  up -d
```

| Overlay file | When used | Key effect |
|--------------|-----------|------------|
| `deploy/docker-compose.yml` | Always | Base services, dev port mappings, build contexts |
| `deploy/docker-compose.local-tunnel.yml` | Local desktop + dev | **Caddy `local-proxy` on `8090:80`**; `PUBLIC_ORIGIN` overrides |
| `deploy/docker-compose.release.yml` | Release ZIP / `-UseImages` | Pin `ghcr.io/aron-chu/streamclone/*:${IMAGE_TAG}`; frontend nginx baked in image |
| `deploy/docker-compose.prod.yml` | Public VM deploy | Caddy on `80/443` with Let's Encrypt |
| `charts/streamclone` | Hosted Kubernetes deploy | App profiles plus optional Prometheus, Grafana, Loki, InfluxDB |

**Project name:** `streamclone` (Docker Compose `name: streamclone`)

### 2.4 Caddy routing (single-origin contract)

Both `deploy/Caddyfile` and `deploy/Caddyfile.local-tunnel` implement the same path → backend map:

| Path | Backend | Notes |
|------|---------|-------|
| `/v1/auth*`, `/v1/me`, `/v1/logout`, `/v1/followed`, `/v1/ws` | `chat:8080` | WebSocket + loopback auth |
| `/v1/stream`, `/v1/stream/*` | `video:8080` | Stream start/stop |
| `/v1/channels/*/emotes*`, `/v1/emotes*`, `/v1/sets*`, `/v1/seed*` | `emote:8080` | Emote API |
| `/emotes/*` | `minio:9000` | Rendered WebP CDN |
| `/v1/analytics/*` | `analytics:8080` | 15m read/write timeout |
| `/live/*` | `mediamtx:8888` | Injects `Authorization: Bearer streamclone-local-hls-cdn` |
| `/v1/setup-control/*` | `host.docker.internal:9191` | Host-side install wizard API |
| `/v1/clipper/*` | `clipper:8095` | Clip Studio (profile) |
| `/v1/*` (remaining) | `metadata:8080` | Directory, search, channels |
| default | `frontend:80` | React SPA |

**Why port 8090:** Documented as the **only** browser entry for local use. Single origin keeps cookies, WebSocket URLs, HLS paths, and `GET /v1/me` auth checks aligned. Raw ports (`8081`–`8086`) exist for developer bypass only.

**Why `caddy:2` not alpine:** `.kiro/steering/tech.md` documents intermittent Docker DNS WebSocket failures with `caddy:2-alpine`.

**MediaMTX HLS on HTTP localhost:** MediaMTX 1.18+ session cookies break plain HTTP unless `hlsCDNSecret` in `deploy/mediamtx.yml` matches the Bearer header Caddy sends on `/live/*`.

---

## 3. Install Paths & User Journeys

### 3.1 End-user install matrix

| Platform | First install | Daily start | Pause | Full removal |
|----------|---------------|-------------|-------|--------------|
| **Windows (recommended)** | `Streamclone-Setup-v*.exe` or `Install Streamclone.cmd` | `Start Streamclone.cmd` / Desktop shortcut | `Stop Streamclone.cmd` | Settings → Apps, Start menu, or `Uninstall Streamclone.cmd` |
| **macOS** | `launchers/Install Streamclone.command` | `launchers/Start Streamclone.command` | `launchers/Stop Streamclone.command` | `launchers/Uninstall Streamclone.command` |
| **Developers** | `git clone` + `scripts/setup.ps1` or `make setup` | `make start` / `scripts/start-streamclone.ps1` | `make down` / Stop launcher | `scripts/uninstall-streamclone.ps1` |

**Install location:** `%USERPROFILE%\streamclone` (Windows), `~/streamclone` (macOS/Linux release bundles).

**Hard prerequisite:** Docker Desktop running. **Not required:** Git, Go, Node, Twitch login (for watching).

### 3.2 Windows Setup.exe flow

**Spec:** `deploy/installer/streamclone-setup.iss` (Inno Setup, built in CI)

1. Checks Docker Desktop (`docker info`)
2. Extracts release bundle to `%USERPROFILE%\streamclone` (fixed path, no picker)
3. Runs `scripts/install-setup-progress.ps1` headlessly; writes progress to temp file for wizard UI
4. Pulls GHCR images (`--pull missing` on subsequent starts)
5. Runs smoke health checks
6. Creates Start menu + Desktop shortcuts
7. Opens `http://localhost:8090/`

**UX goal:** In-wizard progress during image pull (~1.5–2 GB first time); no extra terminal window.

**Known gap:** Not code-signed → Windows shows "Unknown Publisher" (documented in `docs/install-desktop.md`).

### 3.3 Release packaging pipeline

**Script:** `scripts/package-release.sh`
**CI:** `.github/workflows/release-images.yml` on tag `v*`

Release ZIP/tar.gz contains (no app source):

- `deploy/`, `scripts/`, `migrations/`, `launchers/`
- `.env.dev`, `.env.example`, root `.cmd` launchers
- Auto-generated `deploy/env/release-bundle.env`:
  ```env
  IMAGE_TAG=<version>
  STREAMCLONE_USE_IMAGES=1
  ```

**Published assets per release:**

| Asset | Purpose |
|-------|---------|
| `streamclone-<version>-windows.zip` | Manual Windows extract |
| `streamclone-<version>.tar.gz` | macOS/Linux |
| `Install Streamclone.cmd` | One-file bootstrap (fetches ZIP) |
| `Streamclone-Setup-<version>.exe` | Wizard installer |

**GHCR images published:** `metadata`, `chat`, `video`, `emote`, `analytics`, `frontend`, `clipper`
**Not published:** `scraper` (builds from sibling repo when `--profile scraper`)

Registry: `ghcr.io/aron-chu/streamclone/<service>:${IMAGE_TAG}`
**Requirement:** GHCR packages must be **public** or installs fail without `docker login ghcr.io`.

### 3.4 Product tiers (setup profiles)

Installer **`Streamclone-Setup.exe`** ships **Core Watch** only (`core`). Optional tiers use compose profiles after install.

| Tier | Profile | Compose flag | First-pull size (approx.) | Prerequisites | Works without scraper |
|------|---------|--------------|---------------------------|---------------|------------------------|
| **Core Watch** | `core` (default) | none | ~1.5–2 GB GHCR core stack | Docker Desktop | Directory, HLS playback, chat, emotes, Helix/VOD stream lists, TwitchTracker **summary** stats (avg/peak) |
| **Analytics** | `scraper` | `--profile scraper` | + sibling scraper image (not on GHCR) | Sibling [`streamclone-scraper`](https://github.com/Aron-Chu/streamclone-scraper) repo | Minute-level viewer charts, TwitchTracker sync for Analytics |
| **Clip Studio** | `clipper` | `--profile clipper` | + ~1 GB `clipper` image | Twitch CLI + device login | `/studio` clipping pipeline |
| **Full** | `full` | both profiles | Analytics + Clip Studio pulls | Scraper sibling + Twitch CLI | All optional features |

Env fragments: `deploy/env/profile-{core,scraper,clipper,full}.env`. See [install-desktop.md](./install-desktop.md) for end-user tier table.

### 3.5 Lifecycle semantics

| Action | Containers | Install folder | Volumes (pg, minio, clipper) | `.env` secrets |
|--------|------------|----------------|-------------------------------|----------------|
| **Stop** | Stopped | Kept | Kept | Kept |
| **Start** | Restarted | Kept | Kept | Kept |
| **Uninstall** | Removed | **Deleted** | **Deleted** | **Deleted** |

Docker images remain cached after uninstall (faster reinstall). Optional `uninstall-streamclone.ps1 -PruneImages`.

### 3.6 Developer vs release install timing

| Path | First run | Subsequent start |
|------|-----------|------------------|
| Setup.exe / release ZIP (GHCR pull) | ~3–8 min (network-bound) | ~10–30 s |
| Git clone + local image build | ~10–20 min | ~10–30 s |
| Git clone + `-UseImages` | ~3–8 min | ~10–30 s |

**Documented image sizes (core):** video ~900 MB, emote ~430 MB, other services ~600 MB combined → **~1.5–2 GB** first download.

---

## 4. Infrastructure Decisions (with rationale)

### 4.1 Decision log

| Decision | Choice | Rationale | Tradeoffs |
|----------|--------|-----------|-----------|
| **End-user runtime** | Docker Desktop + Compose | Single prerequisite; no Go/Node on user machine; reproducible stack | Requires ~8 GB RAM; Docker Desktop overhead on Windows |
| **Image distribution** | GHCR pre-built images on `v*` tag | Avoid 10–20 min compile on install; pinned `IMAGE_TAG` in release bundle | Public registry required; scraper excluded |
| **Local entry point** | Caddy on **8090** (not 80) | Avoids privileged port / conflicts; clear "app URL" | Users must use 8090, not 8081 etc. |
| **Single origin** | Caddy path routing | Cookies, WS, HLS, auth aligned; `VITE_*=auto` runtime config | More complex proxy config; MediaMTX CDN secret required |
| **Auth model** | Loopback device-code only; redirect OAuth **removed** | No Twitch redirect URL registration for public deploy; simpler security story | Chat login **only** works on `localhost:8090`, not through tunnels |
| **Session store** | Redis (OAuth tokens, chat pub/sub, hot cache) | Fast fan-out; ephemeral sessions | Another container; must not expose 6379 publicly |
| **Durable store** | PostgreSQL (emotes, analytics, migrations) | ACID, migrations via `migrate` container | Default creds `app:app` — dev only |
| **Emote assets** | MinIO S3-compatible + `/emotes/*` proxy | Cheap self-hosted CDN; WebP variants at `{emote_id}/{scale}.webp` | Default `minioadmin` creds; console on 9001 |
| **HLS relay** | MediaMTX + FFmpeg pipe | Low-latency local relay; Caddy injects CDN auth | RTMP/HLS ports exposed in dev compose |
| **Scraper isolation** | Separate repo + optional profile | Browser automation deadlocks, large deps, Camoufox cache | Not in GHCR; manual sibling clone |
| **Windows installer** | Inno Setup EXE + headless PowerShell | Familiar UX; progress file for UI | Unsigned; fixed install path |
| **Setup control API** | Host process on `:9191` | Wizard can drive install without shell | Host loopback only; proxied via Caddy |
| **Prod deploy target** | Oracle Cloud Always Free VM + Caddy TLS | Documented free-ish path in `deploy/FREE_DEPLOYMENT.md` | Not managed PaaS; operator maintains VM |

### 4.2 Data layer conventions

From `.kiro/steering/tech.md`:

- **PostgreSQL:** emotes, sets, channel mappings, analytics jobs (durable)
- **Redis:** metadata cache, `chat:{login}` pub/sub, `channel:emotes:{login}` dictionaries, OAuth sessions (hot/ephemeral)
- **MinIO:** rendered emote WebP; proxied at `/emotes/*`

### 4.3 Port reference (dev + local-tunnel)

| Port | Service | Expose publicly? |
|------|---------|------------------|
| **8090** | Caddy local-proxy (**browser entry**) | Localhost only |
| 8081–8086 | Go services direct | Dev bypass only |
| 8095 | clipper | Trusted network only |
| 8000 | scraper | Trusted network only |
| 8888 / 1935 | MediaMTX HLS / RTMP | Internal; dev mapped |
| 5432 / 6379 | postgres / redis | **Never** |
| 9000 / 9001 | minio API / console | **Never** (console) |
| 9191 | setup-control (host) | Loopback only |

---

## 5. Security & Auth Model

### 5.1 Viewer vs operator

- **Viewers:** Unauthenticated read (directory, stream start, anonymous chat listen, emote CDN)
- **Chat send:** Twitch OAuth → tokens in Redis → authenticated IRC
- **No first-party username/password system**

### 5.2 Loopback-only dev auth

- Gated by `TWITCH_DEV_TOKEN_IMPORT_ENABLED=true` (default in `.env.dev`)
- Device-code flow + optional prepared-token claim (`make twitch-local-auth`)
- `GET /v1/me` reports `canImportLocalToken` — backend-driven, not a frontend flag
- **Must disable** (`false`) on any deployment reachable outside the machine
- Redirect OAuth endpoints were **removed** — no public Twitch callback URL needed

### 5.3 Other auth surfaces

| Surface | Protection | Risk if misconfigured |
|---------|------------|----------------------|
| Curator emote API | `CURATOR_API_TOKEN` bearer | Default `change-me` or empty = open writes |
| Clipper webhooks | `CLIPPER_WEBHOOK_TOKEN` | Empty = auth skipped |
| `VITE_CLIPPER_TOKEN` | Client-visible in `/config.js` | Treat as public secret |
| Clipper read APIs | Unauthenticated | Do not expose `:8095` publicly |

### 5.4 Acceptable local-dev gaps (not for public deploy)

- Permissive CORS on Go services
- Unauthenticated video/analytics control APIs
- Default compose credentials (`app:app`, `minioadmin`)

Full checklist: `docs/security.md`, `deploy/FREE_DEPLOYMENT.md`

### 5.5 Legal

Uses non-public Twitch/7TV endpoints. Educational/personal use; operator liable for ToS compliance. See `docs/security.md`.

---

## 6. CI/CD & Quality Gates

### 6.1 Workflows

| Workflow | Trigger | What it validates |
|----------|---------|-------------------|
| `.github/workflows/ci.yml` | push/PR to main | gitleaks, `go test`/`vet`/govulncheck, frontend build, npm audit, compose config, image builds, **core smoke** (`scripts/smoke-core.sh`) |
| `.github/workflows/release-images.yml` | tag `v*` | Matrix build/push 7 GHCR images; package ZIP/tar.gz; build Setup.exe |
| `.github/workflows/smoke-scraper.yml` | daily cron | Full scraper profile with sibling repo clone |

### 6.2 Local developer guards

```bash
make install-hooks      # gitleaks + env gate + fmt/vet/tsc
make security-scan      # gitleaks + validate-env
make validate-env       # fail on placeholder secrets
```

### 6.3 Release process (maintainer)

1. Tag `v*` (e.g. `v0.1.4`)
2. CI publishes GHCR images + desktop bundle + Setup.exe
3. Verify GHCR packages are public
4. Smoke: fresh install from Setup.exe on clean Windows VM (recommended manual step)

---

## 7. Deployment Modes Beyond Desktop

### 7.1 Local HTTPS tunnel (`deploy/LOCAL_HTTPS_OAUTH.md`)

- Tunnel target: `http://localhost:8090` (Caddy)
- Recommended: Cloudflare Quick Tunnel
- Set `PUBLIC_ORIGIN`, `PUBLIC_ORIGIN_WS`, `FRONTEND_ORIGIN`, `HLS_PUBLIC_BASE`, `CDN_PUBLIC_BASE`
- **Free ngrok:** browser warning page breaks HLS/WebSocket — unsuitable
- **Chat login does not work through tunnel** — loopback only

### 7.2 Free VM production (`deploy/FREE_DEPLOYMENT.md`)

- Oracle Cloud Always Free Ampere A1 (2–4 OCPU, 8–24 GB RAM)
- DuckDNS or owned domain
- `docker-compose.yml` + `docker-compose.prod.yml`
- Caddy Let's Encrypt on 443
- Firewall: only 22, 80, 443 public

### 7.3 What Streamclone is NOT

- Not deployable to static hosting (Vercel/Netlify/GitHub Pages)
- Not a single binary — requires container orchestration
- Not a managed SaaS — operator owns VM, secrets, compliance

---

## 8. Existing Benchmarks & How to Run Them

The repo includes scripts for measuring install and runtime performance. A reviewer should run these on a **clean machine** (or VM) and compare against the documented targets.

### 8.1 Install benchmark

**Script:** `scripts/benchmark-exe-install.ps1`

```powershell
powershell -File scripts\benchmark-exe-install.ps1 -SetupExe dist\Streamclone-Setup-v0.1.x.exe
```

**Measures:**

- Total silent install time (Setup.exe `/VERYSILENT`)
- Phase timestamps from `streamclone-setup-progress.txt`
- Exit code, install folder presence
- HTTP 200 on `http://localhost:8090/`
- Running `streamclone` containers

**Documented target:** ~3–8 min first install (network-dependent).

### 8.2 HLS cold-start benchmark

**Script:** `scripts/benchmark-hls-start.ps1`

```powershell
powershell -File scripts\benchmark-hls-start.ps1 -Channel jynxzi -Runs 3
```

**Measures:** Time from `POST /v1/stream/start` to manifest `200` on `/live/{channel}/main_stream.m3u8`.

### 8.3 Analytics API load benchmark

**Script:** `scripts/benchmark-analytics-load.ps1`

```powershell
powershell -File scripts\benchmark-analytics-load.ps1 -Login jynxzi -Runs 3
```

**Measures:** Latency and payload size for insights, streams list, stream detail, games endpoints.

### 8.4 Scraper benchmark (optional profile)

**Script:** `scripts/benchmark-scraper.ps1` (delegates to sibling `streamclone-scraper/benchmark_scrape.py`)

**Measures:** TwitchTracker scrape latency under concurrency matrices; supports `-UseDocker` to avoid WSL localhost relay issues.

### 8.5 Suggested benchmark matrix for reviewer

| Scenario | Command / action | Pass criteria (suggested) |
|----------|------------------|---------------------------|
| Fresh Windows install | Setup.exe on clean VM, Docker pre-installed | ≤10 min on 100 Mbps; directory loads; no manual steps |
| Cached restart | Stop → Start launcher | ≤60 s to HTTP 200 |
| Core smoke | `scripts/smoke-core.ps1` | All healthchecks green |
| HLS playback | `benchmark-hls-start.ps1` | Manifest <20 s cold start (channel live) |
| Memory at idle | `docker stats` after start | ≤6 GB RSS total (8 GB machine viable) |
| Uninstall | Uninstall launcher | Folder + volumes gone; port 8090 free |

---

## 9. Known Limitations & Open Questions

### 9.1 Documented limitations

| Area | Issue |
|------|-------|
| Code signing | Setup.exe unsigned → SmartScreen warning |
| GHCR visibility | Private packages break end-user install |
| Scraper | Not in release images; sibling repo; `SCRAPER_MAX_CONCURRENT=1` with ephemeral browser on Windows |
| Chat auth | Loopback only; no tunnel/public deploy login |
| ngrok free tier | Breaks HLS/WS through warning interstitial |
| WSL2 / Windows | `wslrelay` can bind stale localhost; fix via `wsl --shutdown` |
| Clipper | Read paths unauthenticated |
| Doc inconsistency | `FREE_DEPLOYMENT.md` may still mention redirect OAuth in one place — redirect flow was removed |
| Legal | May violate Twitch/7TV ToS |

### 9.2 Questions for the reviewer

**Install UX**

1. Is Docker Desktop the right sole prerequisite for non-technical users, or should we offer a Podman/native path?
2. Is fixed `%USERPROFILE%\streamclone` acceptable vs. user-chosen install dir?
3. Is ~1.5–2 GB first download acceptable? What compression/registry mirror strategies would you recommend?
4. How does Setup.exe compare to alternatives (Electron shell, MSI, winget, Docker Desktop extension)?

**Architecture**

5. Is the single-origin Caddy-on-8090 pattern sound for cookies, WS, and HLS long-term?
6. Should any Go services be merged (e.g. metadata + chat) to reduce container count and RAM?
7. Is Redis + Postgres + MinIO the right split, or is one store overkill for solo/self-host?
8. Is excluding scraper from GHCR the right tradeoff vs. publishing a large browser image?

**Security**

9. Is loopback-only device-code auth sufficient for the "optional login" story?
10. What is the minimum hardening checklist before recommending public VM deploy?
11. Are default compose credentials acceptable if documented, or should setup auto-generate all secrets?

**Operations**

12. Is the Stop/Start/Uninstall lifecycle clear enough for non-technical users?
13. What observability gaps exist for production self-hosters (metrics, log aggregation)?
14. Should CI include a Windows install smoke job (expensive but high value)?

**Cost & scale**

15. What is realistic concurrent viewer capacity on Oracle Always Free (2 OCPU / 8 GB)?
16. When does the architecture break (streams × viewers × emote cache)?

---

## 10. Review Deliverables (what we want back)

Please structure your review as:

1. **Executive summary** (1 paragraph: overall assessment)
2. **Install experience score** (1–10) with justification vs. comparable self-hosted apps
3. **Infrastructure score** (1–10) for local desktop + free VM deploy paths
4. **Top 5 risks** (security, reliability, UX, legal, ops)
5. **Top 5 quick wins** (changes under 1 week)
6. **Top 5 strategic changes** (architecture or distribution shifts)
7. **Benchmark results table** (if you ran the scripts above)
8. **Alternative architectures considered** (e.g. k8s, single container, managed DB, Cloudflare Tunnel as primary deploy)
9. **Comparison matrix** vs. 2–3 analogous projects (if known)

---

## 11. Key File Index

### Install & lifecycle

| Path | Role |
|------|------|
| `docs/install-desktop.md` | End-user install guide |
| `docs/options.md` | Profiles and optional features |
| `deploy/installer/streamclone-setup.iss` | Inno Setup spec |
| `scripts/setup.ps1` / `setup.sh` | Interactive setup |
| `scripts/install-setup-progress.ps1` | Headless Setup.exe backend |
| `scripts/start-streamclone.ps1` | Daily start |
| `scripts/uninstall-streamclone.ps1` | Full teardown |
| `scripts/benchmark-exe-install.ps1` | Install timing benchmark |

### Compose & proxy

| Path | Role |
|------|------|
| `deploy/docker-compose.yml` | Base stack |
| `deploy/docker-compose.local-tunnel.yml` | Caddy :8090 |
| `deploy/docker-compose.release.yml` | GHCR images |
| `deploy/docker-compose.prod.yml` | Production TLS |
| `deploy/Caddyfile.local-tunnel` | Local routing rules |
| `deploy/mediamtx.yml` | HLS/RTMP + CDN secret |

### Environment

| Path | Role |
|------|------|
| `.env.dev` | Dev template (committed) |
| `.env.example` | Reference |
| `.env` | Generated at install (not committed) |
| `deploy/env/profile-*.env` | Profile fragments |
| `deploy/env/release-bundle.env` | Pinned `IMAGE_TAG` in releases |

### CI/CD

| Path | Role |
|------|------|
| `.github/workflows/ci.yml` | PR CI + smoke |
| `.github/workflows/release-images.yml` | Release pipeline |
| `scripts/package-release.sh` | Desktop bundle builder |

### Steering & security

| Path | Role |
|------|------|
| `.kiro/steering/tech.md` | Technical conventions |
| `.kiro/steering/local-auth.md` | Auth guardrails |
| `docs/security.md` | Security model |
| `deploy/FREE_DEPLOYMENT.md` | VM deploy guide |
| `deploy/LOCAL_HTTPS_OAUTH.md` | Tunnel guide |

### Benchmarks

| Path | Role |
|------|------|
| `scripts/benchmark-exe-install.ps1` | Install timing |
| `scripts/benchmark-hls-start.ps1` | Playback cold start |
| `scripts/benchmark-analytics-load.ps1` | Analytics API latency |
| `scripts/benchmark-scraper.ps1` | Scraper throughput |

---

## 12. System Requirements (documented)

| | Minimum | Recommended |
|---|---------|-------------|
| OS | Windows 10/11 64-bit, macOS 12+, Linux + Docker | Same |
| RAM | 8 GB | 16 GB |
| Disk | 5 GB free | 10 GB+ |
| CPU | 4 cores | 6+ cores |
| Network | Stable broadband; ~1.5–2 GB first download | Wired / fast Wi‑Fi |

**VM deploy (testing):** 2 OCPU / 8 GB RAM
**VM deploy (smoother workers):** 4 OCPU / 16–24 GB RAM, 80–150 GB disk

---

*Generated for external infrastructure review. Update this brief when major install or deployment decisions change.*
