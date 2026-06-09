# Streamclone code graph MCP

This folder contains a deterministic code graph pipeline for local agent context.
It uses tree-sitter ASTs to map files, definitions, imports, inheritance/embedding,
and direct call edges into an embedded Kuzu database. No LLM extraction is used.

Agents: read `AGENTS.md` first. Prefer `get_ast_chunk` over full-file reads.

## Quick setup

```sh
make codegraph-install
make codegraph
```

Rebuild the graph after large refactors or when `get_ast_chunk` reports `stale_index: true`.

The graph is stored as a single Kuzu file: `.codegraph/streamclone.kuzu` (Kuzu 0.11+ — not a directory).

## Install (manual)

```sh
python3 -m venv .codegraph/.venv
.codegraph/.venv/bin/python -m pip install -r tools/codegraph/requirements.txt
```

## Build the graph (manual)

```sh
.codegraph/.venv/bin/python tools/codegraph/codegraph_ingest.py \
  --repo . \
  --db .codegraph/streamclone.kuzu
```

The schema is:

- Nodes: `File`, `Class`, `Function`, `Interface`, `ImportModule`
- Edges: `DEFINES`, `CALLS`, `INHERITS_FROM`, `IMPORTS`

## Run the MCP server

```sh
.codegraph/.venv/bin/python tools/codegraph/codegraph_mcp.py \
  --repo "$(pwd)" \
  --db "$(pwd)/.codegraph/streamclone.kuzu"
```

## Cursor IDE

Project MCP config: `.cursor/mcp.json` (Windows — runs via `scripts/codegraph-mcp.ps1` + WSL venv).

Linux/macOS: copy `.cursor/mcp.linux.json.example` to `.cursor/mcp.json`.

Enable the server in Cursor Settings → MCP, then reload the window.

Preflight (Windows): `powershell -File scripts/mcp-preflight.ps1`  
Tool counts (expected): codegraph 6, stack 6, data 5 — verify with `wsl bash scripts/mcp-list-tools.sh`

## Codex CLI entry

```sh
codex mcp add streamclone-codegraph -- \
  "$(pwd)/.codegraph/.venv/bin/python" \
  "$(pwd)/tools/codegraph/codegraph_mcp.py" \
  --repo "$(pwd)" \
  --db "$(pwd)/.codegraph/streamclone.kuzu"
```

The MCP server exposes:

- `get_call_chain(function_name)`: direct and indirect callers/callees up to 3 hops.
- `get_blast_radius(symbol_name)`: files, functions, imports, and inheritance/call edges that directly rely on a symbol.
- `get_ast_chunk(function_name)`: exact source text bounded by the function's tree-sitter node.
- `search_symbols(query, kind?, limit?)`: substring search over symbol names.
- `graph_status()`: database age and indexed counts.
- `rebuild_graph()`: re-run ingest and return summary.

## Stack MCP (`streamclone-stack`)

Read-only local stack probes. See [`tools/stack/README.md`](../stack/README.md).

Tools: `stack_health`, `stack_ports`, `playback_probe`, `twitch_auth_status`, `scraper_probe`, `compose_logs`.

## Data MCP (`streamclone-data`)

Read-only Postgres/Redis on localhost compose. See [`tools/data/README.md`](../data/README.md).

Tools: `data_status`, `postgres_query`, `emote_jobs`, `redis_get`, `redis_channel_emotes`.

## CocoIndex Code

CocoIndex Code is complementary to this graph server. It provides semantic
AST-aware search as MCP via `ccc mcp`, while this server provides deterministic
graph traversals and exact syntactic chunks.
