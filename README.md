# Streamclone

Self-hosted Twitch-style live directory, HLS playback, emote-rich chat, and stream analytics — with a local 7TV-style emote pipeline, no ads, and optional spike-based clipping.

After `make bootstrap`, open **`http://localhost:8090`**. Everything below is what you should see on a healthy local stack.

---

## Directory

Live channel grid with categories and search.

**Home — live channels**

<img src="docs/images/directory.png" alt="Live directory" width="960" />

**Browse by category**

<img src="docs/images/directory-category.png" alt="Directory category filter" width="960" />

**Search channels**

<img src="docs/images/directory-search.png" alt="Directory search" width="960" />

---

## Channel

Player, IRC chat with 7TV / FFZ / Twitch emotes, stats, and tabs.

**Channel workspace (player + chat + sidebar)**

<img src="docs/images/channel-xqc.png" alt="Channel workspace" width="960" />

**Live playback**

<img src="docs/images/channel-live.png" alt="Live channel playback" width="960" />

**Emotes tab**

<img src="docs/images/channel-emotes.png" alt="Channel emotes" width="960" />

**Stats tab**

<img src="docs/images/channel-stats.png" alt="Channel stats" width="960" />

---

## Analytics

Historical streams, minute rollups, chat/emote counts, and optional TwitchTracker viewer sync.

**Past streams**

<img src="docs/images/analytics-xqc-streams.png" alt="Analytics past streams list with synced session selected" width="960" />

**Synced minute rollups**

<img src="docs/images/analytics-xqc-chart.png" alt="Completed analytics chart with viewer, chat, and emote minute rollups" width="960" />

**Initial sync load**

<img src="docs/images/analytics-sync-load.gif" alt="Analytics initial sync loading phases before chart data appears" width="960" />

**TwitchTracker scrape**

<img src="docs/images/analytics-tt-scrape.gif" alt="TwitchTracker viewer scrape progress during analytics sync" width="960" />

---

## Quick start

**Prerequisites:** [Docker](https://docs.docker.com/get-docker/) with `docker compose` (Docker Desktop on Windows/macOS; Docker Engine on Linux).

### Non-developers (recommended)

No terminal commands — see **[docs/install-desktop.md](docs/install-desktop.md)**.

**Windows:**

1. Install [Docker Desktop](https://docs.docker.com/desktop/) and start it.
2. Download **`Install Streamclone.cmd`** from [Releases](https://github.com/Aron-Chu/streamclone/releases/latest) (or double-click it from a downloaded repo ZIP).
3. After install, double-click **Start Streamclone** on your Desktop.

**macOS:**

1. Install Docker Desktop and start it.
2. Extract a release bundle to `~/streamclone`, open **`launchers`**, double-click **`Install Streamclone.command`**.
3. Use **Streamclone Start** in `~/Applications` to run.

Opens **http://localhost:8090** when ready. Stop via **Stop Streamclone** (Windows Desktop) or **Streamclone Stop** (macOS).

### Developers

```sh
git clone https://github.com/Aron-Chu/streamclone.git
cd streamclone
make setup        # interactive wizard; or make setup-core
make smoke        # health checks once services are up
```

**Windows (no make):** `powershell -File scripts/setup.ps1` — legacy `scripts/bootstrap.ps1` is core-only.

### Optional profiles

| Command | What it adds |
|---|---|
| `make up` | Core stack only (default) |
| `make up-scraper` | TwitchTracker scraper for historical viewer charts |
| `make up-full` | Scraper + live clipper / Clip Studio |

Scraper builds from sibling repo [streamclone-scraper](https://github.com/Aron-Chu/streamclone-scraper) — clone it next to this repo, then `make up-scraper`.

**Local proxy:** Open `http://localhost:8090` only. Caddy (`local-proxy`) reverse-proxies all services for same-origin auth, chat WebSocket, and HLS — do not point the browser at raw service ports.

**Optional scraper egress:** `make up-scraper` may use `PROXY_*` vars in your private `.env` for TwitchTracker — configure via [streamclone-scraper](https://github.com/Aron-Chu/streamclone-scraper). Never commit proxy URLs, credentials, or session vault keys.

**Refresh README screenshots** (1920×1080, stack must be up):

```powershell
powershell -ExecutionPolicy Bypass -File scripts/capture-readme-media.ps1
```

Or drop your own PNG/GIF files into `docs/images/` with the same filenames. Use a **1920×1080** browser window at 100% zoom so GitHub does not squash them.

Install hooks: `make install-hooks`.

## Architecture

```
Browser  ──REST──►  Metadata :8081   (directory, search, categories)
         ──REST──►  Video     :8082   (Usher handshake, stream workers)
         ──WS────►  Chat      :8083   (IRC gateway, emote tokenization)
         ──REST──►  Emote     :8084   (CRUD, sets, asset pipeline)
         ──REST──►  Analytics :8086   (rollups, historical sync)
         ──HLS───►  MediaMTX  :8888   (RTMP ingest → HLS edge)
         ──img───►  MinIO     :9000   (emote WebP assets)
```

Caddy on `:8090` proxies everything for local dev. PostgreSQL 16 + Redis 7 behind the services.

## License

MIT — see [LICENSE](LICENSE).

## Legal notice

This project accesses third-party internal endpoints (Twitch GraphQL, Usher, IRC, 7TV) for **educational and personal self-hosting only**. It is **not affiliated with or endorsed by Twitch or 7TV**. Use may violate upstream Terms of Service; **the operator is solely responsible for compliance**.
