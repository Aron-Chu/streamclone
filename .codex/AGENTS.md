# Codex layer (Streamclone)

This file supplements the repository root [`AGENTS.md`](../AGENTS.md). Codex merges both.

**Scope:** Core Twitch replica only. StreamPulse backend/ops → [docs/streampulse-product-boundary.md](../docs/streampulse-product-boundary.md).

## Codex-specific

- **MCP:** run `make codex-setup` once, then `make mcp-setup`. Project MCP lives in `.codex/config.toml` (generated). Verify with `/mcp` in Codex.
- **Skills:** mirrored under `.agents/skills/streamclone/` only (synced via `make codex-sync-skills`).
- **Rules:** shell approval policies in `.codex/rules/` (Starlark).
- **Trust:** project `.codex/` config loads only when this repository is a **trusted project** in Codex.
- **Windows:** MCP servers run via WSL; stack URL is `http://localhost:8090` through Caddy.
- **Extension/portal/backend:** open sibling **streamclone-pulse** or private backend/ops checkouts.

Full setup: [`../docs/CODEX.md`](../docs/CODEX.md).
