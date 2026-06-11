# Streamclone

Self-hosted Twitch-style directory with HLS playback, emote-rich chat, analytics, and optional Clip Studio — no ads, local Docker stack at **`http://localhost:8090`**.

## Quick start

**Prerequisites:** [Docker Desktop](https://docs.docker.com/desktop/) (or Docker Engine + Compose on Linux).

| Platform | First install | Every day |
|----------|---------------|-----------|
| **Windows** | Double-click **`Install Streamclone.cmd`** | Double-click **`Start Streamclone.cmd`** (Desktop shortcut after install) |
| **macOS** | **`launchers/Install Streamclone.command`** | **`launchers/Start Streamclone.command`** |
| **Developers** | `make setup` or `scripts/setup.ps1` | `make start` or `scripts/start-streamclone.ps1` |

Browser opens **`/welcome`** — live status for core, scraper, and clipper. Then **Go to directory** when ready.

Details: [docs/install-desktop.md](docs/install-desktop.md) · [docs/getting-started.md](docs/getting-started.md)

**Profiles:** `core` (default) · `scraper` (TwitchTracker charts) · `clipper` (Clip Studio) · `full`

---

## See it in action

3 GIFs + 1 screenshot (1920×1080). Regenerate with stack up:

```powershell
powershell -File scripts/capture-readme-media.ps1
```

**Directory**

<img src="docs/images/directory.gif" alt="Live channel directory" width="960" />

**Channel — live playback + chat**

<img src="docs/images/channel.gif" alt="Channel player and chat" width="960" />

**Analytics — sync in progress**

<img src="docs/images/analytics-sync-load.gif" alt="Analytics VOD sync loading chat and rollups" width="960" />

**Analytics — finished chart**

<img src="docs/images/image.png" alt="Synced analytics chart with viewers, chat, emotes, and top moments" width="960" />

---

## Stack

```
Browser :8090 → Caddy → frontend, metadata, video, chat, emote, analytics, MediaMTX (HLS), MinIO
```

PostgreSQL + Redis behind the Go services. Use **`http://localhost:8090`** only (same-origin auth, chat WS, HLS).

## License

MIT — [LICENSE](LICENSE). Not affiliated with Twitch or 7TV; self-hosting is your responsibility.
