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

Core Watch is installed by default. Optional tiers can be started from the app or setup profiles:

| Tier | Adds | How to start |
|------|------|--------------|
| Analytics | minute-level TwitchTracker charts | **Start Analytics** or scraper profile |
| Clip Studio | `/studio` clip workflow | **Start Clip Studio** or clipper profile |
| Pulse | Grafana/Influx dashboards | **Start Pulse** or pulse profile |
| Full | Analytics + Clip Studio | full profile |

Details: [options.md](options.md). Scraper details: [scraper-cloudflare-and-proxy.md](scraper-cloudflare-and-proxy.md).

## Twitch Sign-In

Not required for watching. Sign in only for chat send, follows, or Clip Studio.

1. Open `http://localhost:8090/`.
2. Click **Sign in (optional)**.
3. Approve on Twitch.

Official releases can include bundled OAuth config. Developers can copy `deploy/env/oauth-bundle.env.example` to `deploy/env/oauth-bundle.env`.

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

## Troubleshooting

- App up but charts empty: start Analytics.
- Clip Studio auth errors: sign in on localhost, then restart Clip Studio.
- `localhost:8090` down: run **Check Streamclone** or `make ps`.
- Install stale after a release: use manager **Update**.
- Public or tunnel exposure: read [security.md](security.md) first.
