# Install Streamclone

Use `http://localhost:8090/` as the app URL. Do not browse raw service ports like `:8081`.

## Requirements

- Docker Desktop running
- Windows 10/11, macOS 12+, or Linux with Docker
- 8 GB RAM minimum, 16 GB recommended
- 5 GB free disk minimum, 10 GB recommended

Watching does not require Git, Go, Node, or Twitch login.

## Windows Install

1. Download `Streamclone-Setup-v*.exe` from [Releases](https://github.com/Aron-Chu/streamclone/releases/latest).
2. Run it. The installer extracts to `%USERPROFILE%\streamclone`, pulls images, starts Docker Compose, and creates the **Streamclone** shortcut.
3. Open `http://localhost:8090/`.

Windows may show "Unknown Publisher" because the installer is not code-signed. If antivirus blocks the EXE, use `Install Streamclone.cmd` from the release ZIP.

## Daily Use

Use the **Streamclone** shortcut to open the manager.

| Action | Keeps data? | Notes |
|--------|-------------|-------|
| Start | Yes | Starts containers and opens the app |
| Stop | Yes | Stops containers only |
| Check | Yes | Read-only diagnostics |
| Repair | Yes | Re-pulls images and recreates containers |
| Update | Yes | Syncs `.env` `IMAGE_TAG` to bundle `VERSION` |
| Reset config | Yes | Removes `.env`, keeps volumes |
| Uninstall | No | Removes containers, volumes, `.env`, shortcut, and install folder |

If Docker Desktop is not running during uninstall, Streamclone can defer Docker cleanup and add a **Finish Streamclone Docker cleanup** shortcut.

## Optional Features

Core Watch is installed by default (directory, playback, IRC chat, emotes). **Minute analytics and Pulse live coverage** live on [StreamPulse](https://streampulse.stream) — not bundled in the desktop install.

| Add-on | What it does | How to start |
|--------|----------------|--------------|
| ReplayForge (Clip Studio) | `/studio` clip workflow | Install [ReplayForge](../replayforge) separately — API `:8095`, UI `:8096` |

Details: [options.md](options.md).

ReplayForge runs outside Streamclone. The browser may ask once to open the registered `streamclone://` link when using in-app setup controls; choose allow/remember. If the prompt is blocked, run **Start Streamclone** once from the Desktop shortcut.

## Twitch Sign-In

Not required for watching. Sign in only for chat send, follows, or Clip Studio exports.

1. Open `http://localhost:8090/`.
2. Click **Sign in (optional)**.
3. Approve on Twitch.

Official releases may include bundled OAuth (`oauth-bundle.env`) for Sign in and optional Twitch clip ingest on Pulse Wire. Without it, **Pulse Wire still works on Reddit** (public/scraper paths when `REDDIT_COMMERCIAL_OK=true`). Self-builders: copy `deploy/env/oauth-bundle.env.example` or run `twitch configure` then setup.

## macOS / Linux

1. Download `streamclone-*.tar.gz` from Releases.
2. Extract to `~/streamclone`.
3. Run the install launcher.
4. Use start/stop/uninstall launchers.

## Developer Checkout

```sh
git clone https://github.com/Aron-Chu/streamclone.git
cd streamclone
make setup
make up
```

Run release-style installs from `%USERPROFILE%\streamclone`; run unreleased source changes from this git checkout.

### Docker Compose (developers)

Always pass the repo `.env` when running Compose from a checkout so service env vars resolve consistently:

```powershell
docker compose --env-file .env -f deploy/docker-compose.yml up -d
docker compose --env-file .env -f deploy/docker-compose.yml ps
docker compose --env-file .env -f deploy/docker-compose.yml logs storygraph
```

```sh
docker compose --env-file .env -f deploy/docker-compose.yml up -d
```

`make up` / `make down` wrap the same flags. Without `--env-file .env`, interpolated values like `REDDIT_PROVIDER` and `SCRAPER_API_URL` may fall back to compose defaults.

## Troubleshooting

- App up but charts empty: start Analytics.
- Clip Studio auth errors: sign in on localhost (chat OAuth syncs clipper tokens), then restart ReplayForge.
- `localhost:8090` down: run **Check Streamclone** or `make ps`.
- Install stale after a release: use manager **Update**.
- Public or tunnel exposure: read [security.md](security.md) first.
