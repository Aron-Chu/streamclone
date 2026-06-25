# Agent code graph (Kuzu)

Deterministic tree-sitter index of the Streamclone checkout for MCP symbol lookup, call chains, routes, tests, and subsystem context.

**Scope:** This repo only. The sibling `streamclone-pulse` extension is not indexed.

---

## Quick commands

```bash
make codegraph-install   # once: venv + pip deps (kuzu==0.11.3)
make codegraph           # full rebuild (~2 min) → .codegraph/streamclone.kuzu
make codegraph-full      # alias for full rebuild
make codegraph-smoke     # verify DB, counts, symbol search, MCP tools
make codegraph-incremental  # experimental: rebuild when manifest detects changes
make mcp-setup           # install + rebuild + scripts/mcp-preflight.sh
```

**Windows:** run via WSL (codegraph venv is Linux):

```powershell
wsl.exe --cd /mnt/c/Users/Aron/twitch-7tv-clone bash -lc 'make codegraph'
wsl.exe --cd /mnt/c/Users/Aron/twitch-7tv-clone bash -lc 'make codegraph-smoke'
```

MCP launch: `scripts/codegraph-mcp.sh` (Cursor/Codex recommended config uses this path unchanged).

---

## MCP tools (`streamclone-codegraph`)

| Tool | Purpose |
|------|---------|
| `search_symbols` | Substring search over functions/classes/interfaces/imports |
| `get_ast_chunk` | Source slice by symbol name **or** `file_path` + `start_line` (+ optional `end_line`) |
| `get_call_chain` | Caller/callee graph to depth 3 |
| `get_blast_radius` | Files/functions/imports touching a symbol |
| `graph_status` | DB age and node counts |
| `rebuild_graph` | Run ingest from MCP |
| `find_callers` | Incoming `CALLS` edges |
| `find_callees` | Outgoing `CALLS` edges |
| `find_routes` | HTTP routes (Go chi + handler linkage) |
| `find_tests_for_symbol` | `*_test.go` / `*.test.ts` links |
| `impact_analysis` | Blast radius + routes + tests + services |
| `explain_subsystem` | Keyword → `Service` seeds from `tools/codegraph/subsystems.json` |

### Examples

```text
search_symbols("mergeMinuteRollups")
  → internal/analytics/api.go:560

get_ast_chunk("mergeMinuteRollups")
get_ast_chunk(file_path="internal/analytics/api.go", start_line=560, end_line=610)

find_routes(path="/v1/extension")
impact_analysis("mergeMinuteRollups")
explain_subsystem("vod sync")
```

---

## Graph schema (summary)

**Core (tree-sitter):** `File`, `Function`, `Class`, `Interface`, `ImportModule`; rels `DEFINES`, `IMPORTS`, `CALLS`, `INHERITS_FROM`.

**Domain (extractors):** `Route`, `Test`, `Service`; rels `HANDLES`, `TESTS`, `BELONGS_TO`, `CLIENT_CALLS`.

Subsystem seeds: [`tools/codegraph/subsystems.json`](../tools/codegraph/subsystems.json) (from [`SERVICE_MAP.md`](SERVICE_MAP.md)).

---

## Layout

```
tools/codegraph/
  codegraph_ingest.py   # Makefile/CI entry shim
  codegraph_mcp.py      # FastMCP server
  ingest.py             # builder
  schema.py, store.py, walker.py, query.py
  extractors/           # treesitter + domain (routes, tests, services)
  smoke.py              # post-build verification
  incremental.py        # experimental manifest (index.sqlite)
  subsystems.json
.codegraph/             # gitignored: venv, kuzu DB, index.sqlite
```

---

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `database not found` | `make codegraph` |
| MCP preflight fails on imports | `make codegraph-install` in WSL |
| Stale `get_ast_chunk` | Rebuild; check `stale_index` in response |
| `make codegraph` slow | Expected full rebuild; incremental is experimental |
| Wrong symbol line | Re-run `make codegraph` after edits |

Preflight: `bash scripts/mcp-preflight.sh` (from repo root, WSL).

---

## CI

`.github/workflows/ci.yml` job `codegraph` runs `make codegraph` then `make codegraph-smoke` when `internal/`, `frontend/src/`, `cmd/`, or `tools/codegraph/` change.

---

## Remaining gaps

- No indexing of `streamclone-pulse` sibling repo
- Go interface / dynamic dispatch resolution is heuristic
- TS references are tree-sitter level, not LSP-grade
- Incremental mode currently falls back to full rebuild when any file changes
- Vector / embedding search not implemented
