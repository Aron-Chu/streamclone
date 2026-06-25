---
name: context-retrieval
description: Chooses the cheapest context source for Streamclone tasks — codegraph for symbols, runtime snapshots for live state, Repomix only as last resort. Use when the agent needs broad codebase context or runtime truth before editing.
---

# Context retrieval ladder

Use **one** layer at a time. Stop when the question is answered.

## 1. Symbols / blast radius (default for code)

`streamclone-codegraph` MCP:

- `search_symbols(query)` → `get_ast_chunk(symbol)` → `get_blast_radius(symbol)`

**Do not** use codegraph for DB row counts, Grafana, or route health.

## 2. Runtime truth (live stack)

Prefer MCP when session has it:

- `stack_health`, `stack_ports`, `playback_probe`, `postgres_query` (SELECT only)

Or generate snapshots (offline-friendly summaries):

```bash
make context-snapshots   # writes runtime/context/*.txt (gitignored)
```

Read **only** the relevant snapshot file — never paste all four into chat.

| File | When |
|------|------|
| `runtime/context/routes.txt` | Caddy routing / 404 / wrong service |
| `runtime/context/db_schema.txt` | migrations, missing columns |
| `runtime/context/backfill_status.txt` | scraper/analytics/pulse backfill queues |
| `runtime/context/grafana.txt` | Pulse dashboards (optional profile) |

## 3. Optional semantic index

**CocoIndex** (if you installed MCP locally): fuzzy "how does X work" across docs + code.
Codegraph still wins for exact Go/TS symbols.

## 4. Repomix (last resort)

Only when codegraph + targeted `Read`/`Grep` cannot locate files:

```bash
npx repomix --config repomix.config.json
```

Output: `runtime/context/repomix-output.xml` — **summarize** sections needed; never dump whole file to user chat.

## Anti-patterns

- Repomix or CocoIndex before a single `get_ast_chunk` call
- Pasting snapshot JSON from `/v1/extension/pulse` in full
- Loading all alwaysApply rules mentally — glob rules apply only when matching files are in scope
