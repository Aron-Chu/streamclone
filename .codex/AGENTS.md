# Codex layer (Streamclone)

This file supplements the repository root [`AGENTS.md`](../AGENTS.md). Codex merges both.

## Codex-specific

- **MCP:** run `make codex-setup` once, then `make mcp-setup`. Project MCP lives in `.codex/config.toml` (generated). Verify with `/mcp` in Codex.
- **Skills:** mirrored under `.agents/skills/streamclone/` (synced from `.cursor/skills/streamclone/` via `make codex-setup`). Invoke with `$skill-name` or let Codex match descriptions.
- **Rules:** shell approval policies in `.codex/rules/` (Starlark). User overrides in `~/.codex/rules/`.
- **Trust:** project `.codex/` config, hooks, and rules load only when this repository is a **trusted project** in Codex.
- **Windows:** Streamclone MCP servers run via WSL (`wsl.exe`); stack URL is `http://localhost:8090` through Caddy.
- **Pulse extension UI:** design PNGs + [`../streamclone-pulse/docs/pulse-extension/figma-handoff.md`](../streamclone-pulse/docs/pulse-extension/figma-handoff.md) (or local mirror `docs/pulse-extension/figma/`). Codex can view PNGs without Figma MCP. Optional live bridge: `figma-bridge` in `.codex/config.toml` + Figma desktop plugin.

Full setup: [`docs/CODEX.md`](../docs/CODEX.md).
