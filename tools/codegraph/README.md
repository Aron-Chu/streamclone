# Code Graph MCP

Deterministic AST graph for Streamclone. Prefer it over broad file reads.

Full agent guide: [`docs/agent-codegraph.md`](../../docs/agent-codegraph.md).

## Setup

```sh
make codegraph-install
make codegraph          # full rebuild (default)
make codegraph-smoke    # verify graph + MCP tools
```

Database: `.codegraph/streamclone.kuzu`.

Experimental: `make codegraph-incremental` (manifest-based; falls back to full rebuild on schema changes).

## Run

```sh
.codegraph/.venv/bin/python tools/codegraph/codegraph_mcp.py \
  --repo "$(pwd)" \
  --db "$(pwd)/.codegraph/streamclone.kuzu"
```

Windows Cursor uses `scripts/codegraph-mcp.ps1`.

## Tools

Legacy:

- `get_ast_chunk(function_name)` or `get_ast_chunk(file_path=..., start_line=..., end_line=...)`
- `get_blast_radius(symbol_name)`
- `get_call_chain(function_name)`
- `search_symbols(query)`
- `graph_status()`
- `rebuild_graph()`

Domain retrieval:

- `find_callers(symbol_or_query)`
- `find_callees(symbol_or_query)`
- `find_routes(query?, method?, path?)`
- `find_tests_for_symbol(symbol_or_file)`
- `impact_analysis(symbol_or_file_or_config)`
- `explain_subsystem(query)`

Stack and data MCP docs: `tools/stack/README.md`, `tools/data/README.md`.
