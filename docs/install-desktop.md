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

### Emote storage moved from MinIO to Silo

MinIO no longer publishes the `minio/minio` image, so emote storage now runs on `pgsty/silo`, a maintained MinIO fork. When you install this release over an existing install, it starts Silo on your existing `minio-data` volume, with no export needed. **Update** and **Repair** do the same. On first start, Silo can update the store's internal metadata, so going back to MinIO needs a backup taken before the switch.

**Back up before you install this release over an existing install.** **Stop** Streamclone. Then run this in PowerShell on Windows, or a terminal on macOS/Linux, from a folder outside the install folder such as Documents (**Uninstall** deletes the install folder). Command Prompt doesn't work, because it doesn't expand `${PWD}`.

```sh
docker run --rm -v streamclone_minio-data:/data:ro -v "${PWD}:/backup" alpine tar czf /backup/minio-data-backup.tgz -C /data .
```

**To restore the backup**, **Stop** Streamclone and run this from the folder that holds `minio-data-backup.tgz`. It reads the whole archive first and changes nothing if the file is missing or damaged. Restoring replaces everything in the volume, so anything stored after the backup is lost.

```sh
docker run --rm -v streamclone_minio-data:/data -v "${PWD}:/backup" alpine sh -c "tar tzf /backup/minio-data-backup.tgz >/dev/null && find /data -mindepth 1 -delete && tar xzf /backup/minio-data-backup.tgz -C /data"
```

**To go back to MinIO:** `minio/minio` can no longer be pulled, so this only works if `docker image ls minio/minio` still lists `latest`.

1. **Stop** Streamclone and restore the backup.
2. Before starting again, open `deploy\docker-compose.yml` in the install folder and change the `minio` service's `image:` line to `minio/minio:latest`. Otherwise **Start** runs Silo on the restored data.
3. Use **Start**, not **Repair**. **Repair** pulls every image again, which fails for `minio/minio`. Installing a newer release switches back to Silo.

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

Official releases do **not** ship Twitch OAuth client secrets. Copy `deploy/env/oauth-bundle.env.example` to a local `oauth-bundle.env` (gitignored) or run `twitch configure` then setup. StreamPulse extension and hosted analytics are separate — see [streampulse-product-boundary.md](streampulse-product-boundary.md).

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
docker compose --env-file .env -f deploy/docker-compose.yml logs metadata
```

```sh
docker compose --env-file .env -f deploy/docker-compose.yml up -d
```

`make up` / `make down` wrap the same flags. Without `--env-file .env`, interpolated values like `REDDIT_PROVIDER` and `SCRAPER_API_URL` may fall back to compose defaults.

## Troubleshooting

- App up but directory empty: wait for metadata sync or run **Check Streamclone**.
- Clip Studio auth errors: sign in on localhost (chat OAuth syncs clipper tokens), then restart ReplayForge.
- `localhost:8090` down: run **Check Streamclone** or `make ps`.
- Install stale after a release: use manager **Update**.
- Public or tunnel exposure: read [security.md](security.md) first.
