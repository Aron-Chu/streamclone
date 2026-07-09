# Streamclone — Copilot instructions

Product: **Streamclone** (GitHub: `Aron-Chu/streamclone`) — self-hosted Twitch replica: watch, HLS, chat, emotes, desktop install.

StreamPulse backend, extension BFF, portal, and hosted ops are **not** owned by this repository. See [`docs/streampulse-product-boundary.md`](../docs/streampulse-product-boundary.md).

## Read first

- [`AGENTS.md`](../AGENTS.md) — task router, golden rules, safe commands
- [`docs/MCP.md`](../docs/MCP.md) — codegraph, stack, data MCP tools
- [`docs/SERVICE_MAP.md`](../docs/SERVICE_MAP.md) — core services only

## Golden rules (always)

1. Narrow diffs only — no drive-by refactors.
2. Route browser/probes through Caddy `http://localhost:8090`, not raw service ports.
3. Use **streamclone-codegraph** MCP (`get_ast_chunk`, `get_blast_radius`) before whole-repo grep.
4. Never commit secrets (`.env`, tokens, `.cursor/mcp.json`).
5. Never edit applied migrations — add new files only.
6. Do not add hosted URLs, SSH paths, or production topology to this public tree.

## Context ladder

1. Codegraph MCP → symbol lookup and blast radius
2. Stack MCP → `stack_health`, `stack_ports`, `compose_logs`
3. Data MCP → read-only `postgres_query` (SELECT only)
4. `make context-snapshots` → `runtime/context/*.txt` summaries

## Skills

Domain workflows: [`.agents/skills/streamclone/`](../.agents/skills/) (mirrored from `.cursor/skills/streamclone/` via `make codex-sync-skills`).

Pulse/backend/ops skills live in **streamclone-pulse**, **streampulse-backend**, or **streampulse-ops** — not here.
