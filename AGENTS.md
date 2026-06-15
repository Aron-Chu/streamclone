# Agent Guide

Product name: **Streamclone**. The local folder may still be `twitch-7tv-clone`; the release install is `%USERPROFILE%\streamclone` and is usually not a git checkout.

Start with one steering doc, then use the code graph before broad reads.

## Task Router

| Task | Read first | Code graph |
|------|------------|------------|
| Any code change | `.kiro/steering/tech.md` | `get_ast_chunk` / `get_blast_radius` |
| Product or UI guardrails | `.kiro/steering/product.md` | As needed |
| Roadmap / backlog | `docs/product-roadmap.md` | As needed |
| Channel player / HLS | `.kiro/steering/playback.md` | `get_ast_chunk("Channel")`, `get_ast_chunk("LivePlayerControls")` |
| Clip Studio / clipper | `.kiro/steering/clipper.md` | `get_ast_chunk("ClipStudio")`, `get_ast_chunk("_process")` |
| Analytics / VOD / rollups | `.kiro/steering/analytics.md` | `get_blast_radius("mergeMinuteRollups")`, `get_ast_chunk("SyncProgressPanel")` |
| System health / optional services | `.kiro/steering/windows-dev.md` | `get_ast_chunk("SystemHealthPanel")`, `get_ast_chunk("useOptionalServices")` |
| Scraper / Cloudflare | `.kiro/steering/analytics.md`, `docs/scraper-cloudflare-and-proxy.md` | As needed |
| Local Twitch auth | `.kiro/steering/local-auth.md` | As needed |
| Emotes / 7TV / FFZ | `.kiro/steering/emote-pipeline.md` | As needed |
| Install / uninstall | `docs/install-desktop.md`, `docs/repo-maintenance.md` | Launcher scripts |
| Security / secrets | `SECURITY.md`, `docs/security.md` | As needed |

## Code Graph

Use `streamclone-codegraph` first:

- `get_ast_chunk(symbol)` for exact source.
- `get_blast_radius(symbol)` for affected files and callers.
- `get_call_chain(symbol, depth=2)` for flow.
- `search_symbols(query)` when names are fuzzy.
- `graph_status()` after rebuilds.

Setup:

```sh
make codegraph-install
make codegraph
```

The database is `.codegraph/streamclone.kuzu` and is gitignored.

## Runtime Probes

Browser/API boundary is `http://localhost:8090` through Caddy. Do not point the UI at raw service ports unless intentionally bypassing the proxy.

Stack MCP tools: `stack_health`, `stack_ports`, `playback_probe`, `twitch_auth_status`, `scraper_probe`, `compose_logs`.

Data MCP tools: `emote_jobs`, `redis_channel_emotes`, `postgres_query`.

## Discipline

1. Preserve unrelated dirty work.
2. Read only one steering doc unless the task crosses domains.
3. Summarize payloads; do not paste full JSON.
4. Use narrow tests first, then `make check` for broad validation.
5. For auth, compose, clipper, env, or public deployment changes, read `docs/security.md` and run `make security-scan`.
6. After scraper, analytics, install, OAuth, or large frontend changes, update steering docs and run `make codegraph`.

## Layout

- User docs: `README.md`, `docs/`
- Go services: `cmd/*`, `internal/*`
- Frontend: `frontend/src/`
- Clipper: `clipper/liveclipper/`
- Compose: `deploy/docker-compose*.yml`
- Agent steering: `.kiro/steering/`
