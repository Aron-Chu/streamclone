<p align="center">
  <img src="docs/images/logo.svg" alt="Streamclone" width="72" height="72" />
</p>

# Streamclone

Self-hosted Twitch-style directory with live HLS playback, IRC chat, and 7TV/FFZ emotes. Optional Clip Studio deeplink to sibling **ReplayForge**. Apache-2.0. Not affiliated with Twitch or 7TV.

**StreamPulse** (hosted extension + analytics portal) is a separate product — see [docs/streampulse-product-boundary.md](docs/streampulse-product-boundary.md).

## Start Here

Need: Docker Desktop running.

1. Download `Streamclone-Setup-v*.exe` from [Releases](https://github.com/Aron-Chu/streamclone/releases/latest).
2. Run it.
   If Windows or antivirus blocks the unsigned EXE, use `Install Streamclone.cmd` from the release ZIP instead.

Daily use: launch **Streamclone** from the Desktop shortcut. Stop, repair, update, and uninstall are in the manager menu.

Developers:

```sh
git clone https://github.com/Aron-Chu/streamclone.git
cd streamclone
make setup
make up
make smoke
```

## Docs

- [Install and lifecycle](docs/install-desktop.md)
- [Optional features](docs/options.md)
- [Agents: Streamclone + ReplayForge](docs/agents-streamclone-and-replayforge.md)
- [For AI agents](AGENTS.md) — task router, MCP, testing, safe commands
- [Multi-repo layout](docs/workspace.md)
- [StreamPulse product boundary](docs/streampulse-product-boundary.md)
- [Security](SECURITY.md)
- [Contributing](CONTRIBUTING.md)

## Preview

<img src="docs/images/directory.gif" alt="Live directory" width="720" />

<img src="docs/images/channel.gif" alt="Channel playback, chat, and emotes" width="720" />

Regenerate preview media with `powershell -File scripts\capture-showcase-media.ps1` while the core stack is running.

## Stack

Browser `:8090` → Caddy → Go services (metadata, video, chat, emote), MediaMTX, MinIO, PostgreSQL, and Redis. Optional **ReplayForge** Clip Studio runs on the host at `:8095` (outside compose).

## License

[Apache License 2.0](LICENSE). You are responsible for complying with Twitch, 7TV, and local laws when operating this software.
