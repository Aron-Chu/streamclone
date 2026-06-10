# Agent guide (Streamclone)

Read this file first. Load **one** domain steering doc, then use the code graph MCP before broad file reads.

## Task router

| Task | Read first | Symbol lookup |
|------|------------|---------------|
| Any change | `.kiro/steering/tech.md` | `get_ast_chunk` / `get_blast_radius` |
| Product / UX guardrails | `.kiro/steering/product.md` | — |
| Live clipper / Clip Studio | `.kiro/steering/clipper.md` | `get_ast_chunk("ClipStudio")`, `get_ast_chunk("VideoStage")`, `get_ast_chunk("CaptionOverlayEditor")`, `get_ast_chunk("_process")`, `get_ast_chunk("prepare_emote_assets")` |
| Analytics / rollups / VODs | `.kiro/steering/analytics.md`, `.kiro/specs/vod-chat-pipeline-notes.md` | `get_blast_radius("mergeMinuteRollups")`, `get_ast_chunk("gqlCommentText")`, `get_ast_chunk("hasGoodChatCoverageFromRollups")` |
| Scraper optimization / TT perf | `.kiro/specs/scraper-optimization-notes.md`, `.kiro/steering/analytics.md` | — |
| HLS playback / MediaMTX 401 | `.kiro/steering/playback.md` | `get_call_chain("waitForHLS")`, `get_blast_radius("filterTwitchAdSegments")` |
| Local Twitch auth | `.kiro/steering/local-auth.md` | — |
| Emotes / 7TV / FFZ | `.kiro/steering/emote-pipeline.md` | — |
| Windows / Docker localhost | `.kiro/steering/windows-dev.md` | — |
| Scraper / CDP Bypass / Cloudflare | `.kiro/steering/windows-dev.md`, `.kiro/specs/scraper-optimization-notes.md`, `streamclone-scraper/main.py` | — |
| Full feature specs | `.kiro/specs/<feature>/` | Use for planning, not every bugfix |

## Code graph MCP (`streamclone-codegraph`)

Deterministic AST graph in `.codegraph/streamclone.kuzu`. Prefer graph tools over `grep` + full-file `read`:

1. **`get_ast_chunk(symbol)`** — exact function/method source (smallest useful context).
2. **`get_blast_radius(symbol)`** — files and callers affected by a change.
3. **`get_call_chain(symbol, depth=2)`** — upstream/downstream flow.
4. **`search_symbols(query)`** — find symbols when the exact name is unknown.
5. **`graph_status()`** / **`rebuild_graph()`** — index freshness and rebuild.

Setup: `make codegraph-install` then `make codegraph`. Rebuild after large refactors. See `tools/codegraph/README.md`.

The Kuzu database is a **single file** at `.codegraph/streamclone.kuzu` (not a directory). If MCP fails to start, run `powershell -File scripts/mcp-preflight.ps1`.

## Stack MCP (`streamclone-stack`)

Runtime probes for **`http://localhost:8090`**: `stack_health`, `stack_ports`, `playback_probe`, `twitch_auth_status`, `scraper_probe`, `compose_logs`. See `tools/stack/README.md`.

## Data MCP (`streamclone-data`)

Read-only Postgres/Redis on local compose: `emote_jobs`, `redis_channel_emotes`, `postgres_query`. See `tools/data/README.md`.

## Project skills

Cursor skills under `.cursor/skills/streamclone/` pair steering docs with MCP tools: `stack-debug`, `playback-hls`, `test-by-domain`, `clipper-local`, `analytics-sync`, `emote-pipeline`.

If the graph is missing or stale, run `make codegraph` before debugging cross-package behavior.

## Context discipline

1. Read the **one** steering file for the domain (table above).
2. Resolve symbols via codegraph MCP; read whole files only for config, compose, or new code.
3. Local browser stack: **`http://localhost:8090`** (Caddy proxy). Do not point the UI at raw service ports unless intentionally bypassing the proxy.
4. Summarize API/job payloads in prose — do not paste full JSON into chat.
5. Narrow tests first (`go test ./internal/analytics/...`, `clipper-test`), then broader suites when crossing packages.
6. Debug mode / instrumentation only when the failure is unknown — not for routine fixes.
7. Git commits use [Conventional Commits](https://www.conventionalcommits.org/) — see `CONTRIBUTING.md` (`type(scope): summary`).

## Layout

- Human docs: `README.md` (user-facing) and `CONTRIBUTING.md`
- Go services: `cmd/*` entrypoints, `internal/*` packages
- Frontend: `frontend/src/`
- Clipper (standalone): `clipper/liveclipper/`
- Compose: `deploy/docker-compose.yml` + `deploy/docker-compose.local-tunnel.yml`
- Agent steering: `.kiro/steering/`, specs: `.kiro/specs/`
