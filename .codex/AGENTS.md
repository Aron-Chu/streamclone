# Codex layer (Streamclone)

This file supplements the repository root [`AGENTS.md`](../AGENTS.md). Codex merges both.

## Codex-specific

- **MCP:** run `make codex-setup` once, then `make mcp-setup`. Project MCP lives in `.codex/config.toml` (generated). Verify with `/mcp` in Codex.
- **Skills:** mirrored under `.agents/skills/streamclone/` and `.agents/skills/pulse/` (synced via `make codex-sync-skills`). Invoke with `$skill-name` or let Codex match descriptions.
- **Rules:** shell approval policies in `.codex/rules/` (Starlark). User overrides in `~/.codex/rules/`.
- **Trust:** project `.codex/` config, hooks, and rules load only when this repository is a **trusted project** in Codex.
- **Windows:** Streamclone MCP servers run via WSL (`wsl.exe`); stack URL is `http://localhost:8090` through Caddy.
- **Multi-root:** open `streamclone-pulse-extension.code-workspace` so sibling `streamclone-pulse` docs and extension code resolve.

## Pulse live coverage (read before review or BFF edits)

1. Sibling [`../streamclone-pulse/docs/pulse-extension/live-coverage-requirements.md`](../streamclone-pulse/docs/pulse-extension/live-coverage-requirements.md)
2. [`../docs/roadmapping.md`](../docs/roadmapping.md), [`../docs/tools.md`](../docs/tools.md)
3. Skill: `.agents/skills/pulse/pulse-live-coverage-review/SKILL.md`

Full Codex copy-paste prompt: [`../docs/CODEX.md`](../docs/CODEX.md) § Pulse live coverage architecture review.

**Pulse extension UI:** design PNGs + [`../streamclone-pulse/docs/pulse-extension/figma-handoff.md`](../streamclone-pulse/docs/pulse-extension/figma-handoff.md). Optional: `figma-bridge` in `.codex/config.toml`.

Full setup: [`../docs/CODEX.md`](../docs/CODEX.md).
