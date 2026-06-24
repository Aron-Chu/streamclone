# Streamclone — Copilot instructions

Product: **Streamclone** (GitHub: `Aron-Chu/streamclone`). Pulse extension spec lives in sibling `streamclone-pulse`.

## Read first

- [`AGENTS.md`](../AGENTS.md) — task router, golden rules, safe commands
- [`docs/MCP.md`](../docs/MCP.md) — codegraph, stack, data MCP tools
- Pulse portal guardrails: `../streamclone-pulse/docs/website-portal/design.md`

## Golden rules (always)

1. Narrow diffs only — no drive-by refactors.
2. Route browser/probes through Caddy `http://localhost:8090`, not raw service ports.
3. Use **streamcloneCodegraph** MCP (`get_ast_chunk`, `get_blast_radius`) before whole-repo grep. Same server as Cursor/Codex `streamclone-codegraph` — see [`docs/MCP.md`](../docs/MCP.md#server-id-aliases).
4. Never commit secrets (`.env`, tokens, `.cursor/mcp.json`).
5. Never edit applied migrations — add new files only.
6. Pulse: backend is source of truth for coverage, backfill, and peaks — no client-side scoring.

## Context ladder

1. Codegraph MCP → symbol lookup and blast radius
2. Stack MCP → `stack_health`, `stack_ports`, `compose_logs`
3. Data MCP → read-only `postgres_query` (SELECT only)
4. `make context-snapshots` → `runtime/context/*.txt` summaries

## Custom agents

Specialized read-only reviewers live in [`.github/agents/`](agents/). Pick them from the Copilot agent dropdown when reviewing Pulse BFF, BearHost ops, or extension/portal UX.

## Skills

Domain workflows are in [`.agents/skills/`](../.agents/skills/) (mirrored from `.cursor/skills/` via `make codex-sync-skills`).
