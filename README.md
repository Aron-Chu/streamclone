<p align="center">
  <img src="docs/images/logo.svg" alt="Streamclone" width="72" height="72" />
</p>

# Streamclone

Self-hosted Twitch-style directory — HLS playback, chat, 7TV emotes, analytics, optional Clip Studio. **Apache-2.0** · not affiliated with Twitch or 7TV.

## Quick start

**Need:** [Docker Desktop](https://docs.docker.com/desktop/) running.

1. Get **`Streamclone-Setup-*.exe`** from **[Releases](https://github.com/Aron-Chu/streamclone/releases/latest)**
2. Run installer → open **`http://localhost:8090/`**
3. Daily: **Start Streamclone** · Stop / uninstall via Desktop shortcuts or launchers

| | Windows | macOS / Linux dev |
|--|---------|-------------------|
| Install | Setup.exe or `Install Streamclone.cmd` | [install-desktop.md](docs/install-desktop.md) |
| Develop | `git clone` + `make setup` | same |

**Docs:** [Install](docs/install-desktop.md) · [Options](docs/options.md) · [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md)

## Preview

<img src="docs/images/directory.gif" alt="Directory" width="720" /> · <img src="docs/images/channel.gif" alt="Channel" width="720" />

Regenerate: `make docs-media` (stack must be up)

## Stack

Browser `:8090` → Caddy → Go services (metadata, video, chat, emote, analytics) + MediaMTX + MinIO · Postgres + Redis

## License

[Apache License 2.0](LICENSE) — open source. You are responsible for compliance with Twitch/7TV terms when operating this software.
