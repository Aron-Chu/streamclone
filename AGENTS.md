# Agent Guide

Product name: **Streamclone** (GitHub: `Aron-Chu/streamclone`). The local checkout folder may still be named `twitch-7tv-clone`; the release install lives at `%USERPROFILE%\streamclone` and is usually **not** this git repo. Do not refer to the product as `twitch-7tv-clone` in docs or commits.

**Deep references:** [Service map](docs/SERVICE_MAP.md) · [Testing](docs/TESTING.md) · [MCP](docs/MCP.md) · [Codex](docs/CODEX.md) · [Environment](docs/ENVIRONMENT.md) · [Workspace layout](docs/workspace.md)

---

## Golden rules

1. **Preserve unrelated dirty work** — narrow diffs only; no drive-by refactors.
2. **No product behavior changes** unless the task explicitly asks for them.
3. **Read one steering doc** (see Task router) before broad file reads.
4. **Use the code graph MCP** before grepping the whole tree (`get_ast_chunk`, `get_blast_radius`).
5. **Route through Caddy** — browser and probes use `http://localhost:8090`, not raw service ports.
6. **Summarize API payloads** — do not paste full JSON into chat.
7. **Never commit secrets** — `.env`, tokens, `oauth-bundle.env`, machine-specific `.cursor/mcp.json`.
8. **Never edit applied migrations** — add new migration files only.
9. **Run narrow tests first**, then `make check-quick` or `make check` before a PR.
10. **Update steering/docs** after scraper, analytics, Pulse Wire, install, OAuth, or large frontend changes; run `make codegraph` when symbols move.

---

## Quick start

```sh
make up              # core stack (Caddy :8090)
make ports           # host port summary
make smoke           # API smoke (stack must be up)
```

Windows localhost issues → `.kiro/steering/windows-dev.md`.

```sh
make check-quick     # fast pre-PR gate (Go + frontend tests + compose config)
make check           # full gate (adds security-scan, audit, clipper, build)
make mcp-setup       # codegraph + mcp preflight (see docs/MCP.md)
make codex-setup     # Codex: .codex/config.toml + .agents/skills sync (see docs/CODEX.md)
bash scripts/mcp-preflight.sh              # verify MCP stdio (Linux/WSL)
```

---

## Task router

| Task | Read first | Code graph / probes |
|------|------------|---------------------|
| Any code change | `.kiro/steering/tech.md` | `get_ast_chunk`, `get_blast_radius` |
| Product / UI guardrails | `.kiro/steering/product.md` | As needed |
| Roadmap / backlog | `README.md`, `.kiro/steering/product.md` | As needed |
| **Playback / HLS** | `.kiro/steering/playback.md`, `docs/low-latency-relay/requirements.md` | `get_ast_chunk("Channel")`, `playback_probe` |
| **Analytics / VOD / rollups** | `.kiro/steering/analytics.md` | `get_blast_radius("mergeMinuteRollups")`, `get_ast_chunk("SyncProgressPanel")` |
| **Pulse extension / pulse-core / BFF** | [docs/workspace.md](docs/workspace.md), `docs/pulse-extension/` (redirect), sibling [streamclone-pulse `docs/pulse-extension/`](https://github.com/Aron-Chu/streamclone-pulse/tree/master/docs/pulse-extension), `internal/analytics/extension_api.go`, `packages/pulse-core/` | `get_ast_chunk("ExtensionRoutes")`, `curl :8090/v1/extension/health` |
| **Scraper / Cloudflare / proxy** | `.kiro/steering/analytics.md`, `docs/scraper-cloudflare-and-proxy.md`, `docs/scraping-archive/requirements.md` | `scraper_probe`, `make scraper-preflight` |
| **Pulse Wire / storygraph** | `.kiro/steering/pulse-wire.md`, `docs/options.md`, `docs/tiers-scraper-and-social-spread.md` | `get_ast_chunk("PulseWirePage")`, `get_blast_radius("ingestAll")` |
| **Emotes / 7TV / FFZ** | `.kiro/steering/emote-pipeline.md` | `emote_jobs`, `redis_channel_emotes` |
| **Local Twitch auth** | `.kiro/steering/local-auth.md` | `twitch_auth_status`, `make twitch-debug` |
| **Install / desktop / bootstrap** | `docs/install-desktop.md`, `docs/repo-maintenance.md` | Launcher scripts under `scripts/` |
| **Laptopworker dev hub (Tailscale)** | [`docs/laptopworker-dev.md`](docs/laptopworker-dev.md) | `ssh aron@laptopworker`, `scripts/laptopworker-remote.ps1`; extension/portal backends in runbook § Dev workflows |
| **Release / CI / images** | `.github/workflows/release-images.yml`, `VERSION`, `docs/repo-maintenance.md` | `make compose-config-check` |
| Clip Studio / ReplayForge | `docs/agents-streamclone-and-replayforge.md` | Sibling `../replayforge` — not in-repo clipper |
| Clipper (legacy stub) | `.kiro/steering/clipper.md`, `clipper/README.md` | Deprecated in compose |
| System health / optional services | `.kiro/steering/windows-dev.md` | `stack_health`, `get_ast_chunk("SystemHealthPanel")` |
| Scraping archive / Azure blob backfill | `docs/scraping-archive/requirements.md` | `get_ast_chunk("SyncService")` |
| Security / secrets | `SECURITY.md`, `docs/security.md` | `make security-scan` |

---

## Skills router

Load the matching skill from `.cursor/skills/streamclone/` when the task fits (read skill **before** broad file reads). Codex uses the mirror under `.agents/skills/streamclone/` (`make codex-sync-skills` after edits).

| Task | Skill |
|------|-------|
| Agent docs / Makefile / MCP setup | `agent-readiness/SKILL.md` |
| Playback / HLS / latency | `playback-hls/SKILL.md` |
| Analytics sync / rollups / scraper charts | `analytics-sync/SKILL.md` |
| Scraper / Cloudflare / proxy / TT timings | `scraper-debug/SKILL.md` |
| Pulse Wire news / storygraph / LSF/Reddit | `pulse-wire-news/SKILL.md` |
| Stack health / ports / localhost | `stack-debug/SKILL.md` |
| Emotes / 7TV / FFZ | `emote-pipeline/SKILL.md` |
| Clip Studio / ReplayForge | `clipper-local/SKILL.md` |
| Windows install / Setup.exe / release bundle | `release-windows/SKILL.md` |
| Choosing tests by domain | `test-by-domain/SKILL.md` |
| Pulse extension (Chrome MV3) | Cross-read [streamclone-pulse `AGENTS.md`](https://github.com/Aron-Chu/streamclone-pulse/blob/master/AGENTS.md) and [docs/workspace.md](docs/workspace.md) |

---

## Safe commands

| Command | Purpose |
|---------|---------|
| `make up` / `make stop` / `make ps` | Stack lifecycle |
| `make test` / `make vet` | Go unit tests |
| `make test-video` / `test-analytics` / … | Domain-scoped Go tests (see [TESTING.md](docs/TESTING.md)) |
| `make frontend-test` / `make frontend-build` | Frontend checks |
| `make compose-config-check` | Validate compose merges |
| `make check-quick` | Fast combined gate |
| `make smoke` / `make smoke-ui` / `make agent-smoke` | Runtime smoke (needs stack) |
| `make validate-env PROFILE=core` | Env profile sanity |
| `make codegraph` / `make mcp-setup` | Rebuild AST graph / full MCP setup |
| `make scraper-preflight` | Scraper deps / image check |
| MCP: `stack_health`, `playback_probe`, `postgres_query` (SELECT) | Diagnostics |

---

## Commands to avoid without explicit approval

| Command | Risk |
|---------|------|
| `make nuke` / `make down-clean` | Deletes volumes / data |
| `docker volume rm`, `docker system prune` | Data loss |
| `make security-scan` with fix flags that rewrite files | May touch many paths |
| Editing `.env`, `.env.local`, `oauth-bundle.env` | Secrets / machine state |
| Overwriting `.cursor/mcp.json` | Machine-specific paths |
| `git push --force` to `master` | History rewrite |
| Applying `migrate-semantic` or optional migrations without task scope | Schema changes |
| Stopping production-like shared scraper profiles on a shared host | Blocks other devs |

---

## Risky files and services

| Area | Paths / services | Notes |
|------|------------------|-------|
| Secrets | `.env*`, `runtime/`, `oauth-bundle.env` | Never commit; copy from `.env.example` |
| Migrations | `migrations/` | Forward-only; no edits to applied SQL |
| Compose / proxy | `deploy/docker-compose*.yml`, `deploy/Caddyfile*` | Wrong route breaks all APIs |
| Playback | `deploy/mediamtx.yml`, `internal/video/`, `frontend/src/playback.ts` | HLS 401 if CDN secret mismatch |
| Auth | `internal/chat/auth*`, Twitch OAuth env | Session cookies, token import |
| Scraper | `deploy/docker-compose.yml` profile `scraper`, sibling `streamclone-scraper` | Browser pool locks; ephemeral mode on Windows |
| Pulse Wire ingest | `cmd/storygraph`, `internal/storygraph/`, `internal/social/` | Long-running ingest; optional profiles |
| Pulse extension BFF | `internal/analytics/extension_api.go`, bookmarks, recap | Caddy routes `/v1/extension/*`, `/v1/pulse/*` |
| pulse-core | `packages/pulse-core/` | Shared types/scoring; imported by frontend and extension |
| Release | `VERSION`, `.github/workflows/release-images.yml` | Tag push triggers GHCR + Setup.exe |
| Agent config | `.cursor/mcp.json` | Gitignored; use `*.example` only |

---

## Testing matrix (summary)

Full detail: [docs/TESTING.md](docs/TESTING.md).

| Scope | Command |
|-------|---------|
| Fast agent/PR check | `make check-quick` |
| Full PR gate | `make check` |
| Domain map | `.cursor/skills/streamclone/test-by-domain/SKILL.md` |
| Stack smoke | `make smoke`, `make agent-smoke` |
| HLS latency benchmark | `scripts/measure-hls-latency.ps1` |
| Scraper proxy benchmark | `make scraper-proxy-benchmark` |
| Playwright UI | `make smoke-ui`, `frontend/tests/playwright/` |

---

## MCP (summary)

Full detail: [docs/MCP.md](docs/MCP.md). Starter config: [`.cursor/mcp.recommended.json.example`](.cursor/mcp.recommended.json.example) → local `.cursor/mcp.json` (gitignored).

| Server | Use for |
|--------|---------|
| `streamclone-codegraph` | Symbol lookup, call chains, blast radius |
| `streamclone-stack` | Health, ports, playback, auth, scraper, **compose_logs** (read-only logs) |
| `streamclone-data` | Read-only Postgres/Redis, emote jobs |
| **Playwright** (Essential) | UI smoke — `make smoke-ui`, `frontend/tests/playwright/` |

Setup: `make mcp-setup` then copy recommended example. Preflight: `bash scripts/mcp-preflight.sh` (or `scripts/mcp-preflight.ps1` on Windows → WSL).

---

## Docker Compose profiles

Detail: [docs/ENVIRONMENT.md](docs/ENVIRONMENT.md), [docs/SERVICE_MAP.md](docs/SERVICE_MAP.md).

| Profile | Adds | Typical use |
|---------|------|-------------|
| *(default core)* | metadata, video, chat, analytics, emote, frontend, local-proxy, postgres, redis, minio, mediamtx | Daily dev |
| `scraper` | TwitchTracker scraper (`:8000`) | Analytics minute charts |
| `pulse` | InfluxDB, Prometheus, Grafana | Live stats time series |
| `pulse-wire` | storygraph ingest UI/API, media-matcher, x-ingest | Pulse Wire edition |
| `pulse-wire-semantic` | `migrate-semantic` optional PG extensions | Semantic search experiments |
| `clipper` | *(no compose service — Clip Studio = ReplayForge on host :8095)* | Not a compose profile; `profile-clipper.env` is legacy |

Env overlays: `deploy/env/profile-{core,scraper,pulse,full}.env` (+ legacy `profile-clipper.env`). Feature profiles are merged from `.env` via `scripts/lib/env.sh`.

---

## Code graph

Use `streamclone-codegraph` first:

- `get_ast_chunk(symbol)` — exact source slice
- `get_blast_radius(symbol)` — callers and affected files
- `get_call_chain(symbol, depth=2)` — call flow
- `search_symbols(query)` — fuzzy name search
- `graph_status()` — DB freshness

Setup:

```sh
make codegraph-install
make codegraph
```

Database: `.codegraph/streamclone.kuzu` (gitignored).

---

## Layout

- User docs: `README.md`, `docs/`
- Go services: `cmd/*`, `internal/*` (Pulse Wire: `cmd/storygraph`, `internal/storygraph/`, `internal/social/`)
- Frontend: `frontend/src/` (Pulse Wire UI: `frontend/src/components/pulsewire/`)
- Clipper (legacy stub): `clipper/` — active Clip Studio in sibling **ReplayForge** (`../replayforge`); see [docs/workspace.md](docs/workspace.md)
- Pulse extension spec: sibling **streamclone-pulse** (`../streamclone-pulse`); BFF and `packages/pulse-core/` in this repo
- Compose: `deploy/docker-compose*.yml`
- Agent steering: `.kiro/steering/`
