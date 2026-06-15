# Code Graph MCP

Deterministic AST graph for Streamclone. Prefer it over broad file reads.

## Setup

```sh
make codegraph-install
make codegraph
```

Database: `.codegraph/streamclone.kuzu`.

## Run

```sh
.codegraph/.venv/bin/python tools/codegraph/codegraph_mcp.py \
  --repo "$(pwd)" \
  --db "$(pwd)/.codegraph/streamclone.kuzu"
```

Windows Cursor uses `scripts/codegraph-mcp.ps1`.

## Tools

- `get_ast_chunk(function_name)`
- `get_blast_radius(symbol_name)`
- `get_call_chain(function_name)`
- `search_symbols(query)`
- `graph_status()`
- `rebuild_graph()`

Stack and data MCP docs: `tools/stack/README.md`, `tools/data/README.md`.
