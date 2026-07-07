# Agent Guide

Product name: **Streamclone** (GitHub: `Aron-Chu/streamclone`). The local checkout folder may still be named `twitch-7tv-clone`; the release install lives at `%USERPROFILE%\streamclone` and is usually **not** this git repo. Do not refer to the product as `twitch-7tv-clone` in docs or commits.

**Deep references:** [Service map](docs/SERVICE_MAP.md) · [Testing](docs/TESTING.md) · [MCP](docs/MCP.md) · [Codex](docs/CODEX.md) · [Environment](docs/ENVIRONMENT.md) · [Workspace layout](docs/workspace.md)

## Cursor vs generic agents

| Layer | Path | Role |
|-------|------|------|
| **This file** | `AGENTS.md` | Repo router — task table, golden rules, safe commands (Cursor, Codex, etc.) |
| **Cursor rules** | `.cursor/rules/` | Cursor-specific behavior (`streamclone.mdc`, `agents-router.mdc`, domain rules) |
| **Cursor skills** | `.cursor/skills/streamclone/` | Domain skills; Pulse portal skills in sibling **streamclone-pulse** `.cursor/skills/` |
| **Cursor subagents** | `.cursor/agents/` | Backend-safety and ops-diagnostics reviewers (portal UX reviewer in streamclone-pulse) |
| **Cursor hooks** | `.cursor/hooks.json` | Lightweight gofmt/compose/codegraph hints — not full test suites |

Pulse/StreamPulse product docs and portal guardrails: sibling [`streamclone-pulse`](../streamclone-pulse) checkout.

---

## Product scope (2026-07)

Recent scope changes agents should treat as current truth (do not reintroduce removed tiers without an explicit product decision):

| Change | What removed / changed | What remains optional |
|--------|------------------------|------------------------|
| **Ops UI strip** | Local Grafana/Influx/Prometheus compose profile; Stack status / Network panels for those tiers; Channel **Stats** tab and **Pulse** sidebar; **TwitchTracker scraper** tier and minute-chart start prompts | Core stack + ReplayForge link in stack status; in-app session stats only |
| **Pulse Wire removal** | `/pulse-wire` UI, `storygraph` / `x-ingest` / `media-matcher` compose services, `/v1/pulse-wire/*` Caddy routes, `cmd/storygraph`, `internal/storygraph`, `PULSE_WIRE_ENABLED` frontend flag | `internal/social/` Reddit/LSF helpers (metadata); archive `pulsewire/*` cold export paths |
| **UX polish** | — | `content-enter` / `content-stagger` load motion; `useMinSkeletonTime`; horizontal shelf wheel-scroll + softer chrome; channel workspace fade-in |

**Stack status (UI):** core services + ReplayForge link only — not Grafana, not Pulse Wire, not scraper.

**Compose profiles (local dev checkout):** `core` (default). Optional `scraper` / `full` remain in source for operators (`streamclone-scraper` sibling); **desktop install bundles exclude scraper**.

**ReplayForge / auto clipper (2026-07):** ReplayForge is a sibling standalone app, not a production service in this repo and not yet wired as a default hosted service in private **streampulse-ops**. Streamclone owns clip candidates, moment context, trigger routes, mirrored job state, and callback auth; ReplayForge owns render/edit/export. Treat auto clipper as a private-beta backend pipeline until ReplayForge packaging, private ops service wiring, and durable artifact storage/playback are proven. Do not add FFmpeg/render/editor code back into Streamclone.

**Verify after doc/code drift:** `make compose-config-check`, `cd frontend && npx tsc -b && npm test`, `curl http://127.0.0.1:8090/v1/extension/health`.

Historical Pulse Wire notes: [`.kiro/steering/pulse-wire.md`](.kiro/steering/pulse-wire.md) (deprecated pointer only). Full handoff: [`docs/agent-notes/product-scope-2026-07.md`](docs/agent-notes/product-scope-2026-07.md).

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
10. **Update steering/docs** after scraper, analytics, install, OAuth, or large frontend changes; run `make codegraph` when symbols move.
11. **Hosting split** — **Hosted production ops** live in private **streampulse-ops** (deploy by pinned `IMAGE_TAG`, digest promotion). **streamclone** = app source, migrations, local dev, CI, **source** GHCR images. Public API: `https://api.streampulse.stream`. **Pre-cutover:** production may still use `ghcr.io/aron-chu/streamclone/*`; **target:** promoted `ghcr.io/aron-chu/streampulse/*` — see [`docs/production-artifact-contract.md`](docs/production-artifact-contract.md) (source-build), [`docs/production-promotion-contract.md`](docs/production-promotion-contract.md) (hosted promotion), sibling [`streamclone-image-exit-audit-2026-07.md`](../streamclone-pulse/docs/pulse-extension/evidence/streamclone-image-exit-audit-2026-07.md). Launch ledger: [`improvements.md`](../streamclone-pulse/docs/pulse-extension/evidence/improvements.md). **laptopworker** = tailnet core dev only (`docs/laptopworker-dev.md`). Do not run scraper/workers on laptop. Pick smallest workspace from [`docs/workspace.md`](docs/workspace.md). See [`docs/ops-migration-manifest.md`](docs/ops-migration-manifest.md).

## Agent context workflow (required)

Apply this sequence before editing code:

1. Read exactly one domain steering/product doc from the task router.
2. Use codegraph MCP first (`get_ast_chunk`, `get_blast_radius`, `get_call_chain`) to scope symbols and impact.
3. Use stack/data MCP (`stack_health`, `stack_ports`, `compose_logs`, `postgres_query` SELECT only) when runtime truth matters.
4. Use `make context-snapshots` for route/schema/runtime summaries.
5. Use targeted file reads/grep only after MCP + snapshots are exhausted.

Do not start with whole-repo grep unless MCP is unavailable. If MCP is stale/unavailable, run `make mcp-setup` or `make codegraph` first.

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
| Context / runtime truth | [`docs/agent-context.md`](docs/agent-context.md) | `make context-snapshots`, MCP `stack_health` |
| **Watch UI / directory UX** | `.kiro/steering/product.md`, `frontend/src/components/directory/` | Manual `:8090` — stagger, shelf scroll, skeleton timing |
| Roadmap / backlog | `README.md`, `.kiro/steering/product.md` | As needed |
| **Playback / HLS** | `.kiro/steering/playback.md`, `docs/low-latency-relay/requirements.md` | `get_ast_chunk("Channel")`, `playback_probe` |
| **Analytics / VOD / rollups** | `.kiro/steering/analytics.md` | `get_blast_radius("mergeMinuteRollups")`, `get_ast_chunk("SyncProgressPanel")` |
| **Pulse extension / pulse-core / BFF** | [docs/workspace.md](docs/workspace.md), `docs/pulse-extension/` (redirect), sibling [streamclone-pulse `docs/pulse-extension/`](https://github.com/Aron-Chu/streamclone-pulse/tree/master/docs/pulse-extension), `internal/analytics/extension_api.go`, `packages/pulse-core/` | `get_ast_chunk("ExtensionRoutes")`, `curl :8090/v1/extension/health` |
| **Pulse live coverage / VOD backfill / Protect** | sibling [`live-coverage-requirements.md`](../streamclone-pulse/docs/pulse-extension/live-coverage-requirements.md), [`docs/pulse-extension/roster-naming-truth-table.md`](docs/pulse-extension/roster-naming-truth-table.md) (top500 vs live admission vs corpus), [`docs/roadmapping.md`](docs/roadmapping.md), [`docs/tools.md`](docs/tools.md), [`docs/CODEX.md`](docs/CODEX.md) § Pulse review | Skill `pulse-live-coverage-review`, `get_blast_radius("SyncPulseMissedChat")`, `curl :8090/v1/extension/pulse/channels/{login}` |
| **Scraper / Cloudflare / proxy** | `.kiro/steering/analytics.md`, `docs/scraper-cloudflare-and-proxy.md`, `docs/scraping-archive/requirements.md` | `scraper_probe`, `make scraper-preflight` |
| **Emotes / 7TV / FFZ** | `.kiro/steering/emote-pipeline.md` | `emote_jobs`, `redis_channel_emotes` |
| **Local Twitch auth** | `.kiro/steering/local-auth.md` | `twitch_auth_status`, `make twitch-debug` |
| **Install / desktop / bootstrap** | `docs/install-desktop.md`, `docs/repo-maintenance.md` | Launcher scripts under `scripts/` |
| **Laptopworker dev hub (Tailscale)** | [`docs/laptopworker-dev.md`](docs/laptopworker-dev.md), [`.kiro/steering/laptopworker-hosting.md`](.kiro/steering/laptopworker-hosting.md) | `scripts/laptopworker-remote.ps1`; **no scraper/workers on laptop** |
| **Release / CI / images** | `.github/workflows/release-images.yml`, `VERSION`, `docs/repo-maintenance.md` | `make compose-config-check` |
| Clip Studio / ReplayForge / auto clipper | `docs/agents-streamclone-and-replayforge.md` | Sibling `../replayforge`; beta pipeline until ops service + durable artifacts ship |
| Clipper (legacy stub) | `.kiro/steering/clipper.md`, `clipper/README.md` | Deprecated in compose |
| System health / optional services | `.kiro/steering/windows-dev.md` | `stack_health`, `get_ast_chunk("SystemHealthPanel")` |
| Scraping archive / Azure blob backfill | `docs/scraping-archive/requirements.md` | `get_ast_chunk("SyncService")` |
| **Hosted production (operator)** | [`docs/hosted-production-vps.md`](docs/hosted-production-vps.md) (**SSH:** `root` + `~/.ssh/id_ed25519` via Tailscale `hosted-production-vps`), [`docs/ops-migration-truth-table.md`](docs/ops-migration-truth-table.md) (tags vs private ops FAQ), [`docs/production-promotion-contract.md`](docs/production-promotion-contract.md), [`docs/production-artifact-contract.md`](docs/production-artifact-contract.md), sibling [image exit audit](../streamclone-pulse/docs/pulse-extension/evidence/streamclone-image-exit-audit-2026-07.md), private **streampulse-ops**, sibling [`improvements.md`](../streamclone-pulse/docs/pulse-extension/evidence/improvements.md) | `curl https://api.streampulse.stream/v1/extension/health`, `bash scripts/hosted-launch-probes.sh`, `bash scripts/ops/ssh-access-preflight.sh` |
| **BearHost rollback (operator)** | private **streampulse-ops** `archive/bearhost/` | stub: [`docs/bearhost-production.md`](docs/bearhost-production.md) |
| **Azure archive → R2 migration / storage SoT** | [`docs/storage/README.md`](docs/storage/README.md), [`docs/storage/azure-to-r2-migration.md`](docs/storage/azure-to-r2-migration.md) | Read-only inventory: `scripts/storage/azure-prefix-inventory.sh` |
| Security / secrets | `SECURITY.md`, `docs/security.md` | `make security-scan` |

---

## Skills router

Load the matching skill from `.cursor/skills/streamclone/` when the task fits (read skill **before** broad file reads). Codex uses mirrors under `.agents/skills/` (`make codex-sync-skills` after edits): `streamclone/`, `pulse/`, `workflow/` (brainstorming, TDD, debugging, plans, verification).

| Task | Skill |
|------|-------|
| Agent docs / Makefile / MCP setup | `agent-readiness/SKILL.md` |
| Claude Code setup (mirror Cursor MCP/skills/agents) | [`docs/CLAUDE.md`](docs/CLAUDE.md) — `make claude-setup` |
| Playback / HLS / latency | `playback-hls/SKILL.md` |
| Analytics sync / rollups / scraper charts | `analytics-sync/SKILL.md` |
| Scraper / Cloudflare / proxy / TT timings | `scraper-debug/SKILL.md` |
| Pulse Wire news / storygraph / LSF/Reddit | *(removed — see git history before 2026-07)* |
| Stack health / ports / localhost | `stack-debug/SKILL.md` |
| Emotes / 7TV / FFZ | `emote-pipeline/SKILL.md` |
| Clip Studio / ReplayForge | `clipper-local/SKILL.md` |
| Windows install / Setup.exe / release bundle | `release-windows/SKILL.md` |
| Choosing tests by domain | `test-by-domain/SKILL.md` |
| Context ladder (codegraph → snapshots → Repomix) | `context-retrieval/SKILL.md` |
| Pulse extension (Chrome MV3) | Cross-read [streamclone-pulse `AGENTS.md`](https://github.com/Aron-Chu/streamclone-pulse/blob/master/AGENTS.md) and [docs/workspace.md](docs/workspace.md) |
| Pulse live coverage / VOD backfill / Protect / hosted VPS | `.cursor/skills/pulse/pulse-live-coverage-review/SKILL.md` (+ `coverage-triage`, `backfill-safety-review`, `capacity-governor-review`) — Codex: `.agents/skills/pulse/` |
| StreamPulse portal / website-portal tasks | `streamclone-pulse/.cursor/skills/streamclone-task-runner/` — Codex: `.agents/skills/pulse/streamclone-task-runner/` (`make codex-sync-skills`) |

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
| `make context-snapshots` | Write `runtime/context/*.txt` route/schema/backfill/grafana summaries |
| `make context-verify` | Check rules, hooks, codegraph freshness (no Docker required) |
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
| Social helpers | `internal/social/` | Reddit/LSF helpers for metadata; not a user-facing wire product |
| Pulse extension BFF | `internal/analytics/extension_api.go`, bookmarks, recap | Caddy routes `/v1/extension/*`, `/v1/pulse/*` |
| pulse-core | `packages/pulse-core/` | Shared types/scoring; imported by frontend and extension |
| Release | `VERSION`, `.github/workflows/release-images.yml` | Tag push triggers GHCR + Setup.exe |
| Production promotion | `docs/production-promotion-contract.md`, sibling image exit audit | Pre-cutover: `streamclone/*`; target: promoted `streampulse/*` by digest |
| Agent config | `.cursor/mcp.json` | Gitignored; use `*.example` only |
| VPS SSH (hosted ops) | Operator WSL `~/.ssh/id_ed25519` → `operator-host` | Not in git; BearHost keys rejected; `aron-wsl` not authorized yet — see [`docs/hosted-production-vps.md`](docs/hosted-production-vps.md) |

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
| `streamclone-pulse-codegraph` | Extension/portal TS (`streamclone-pulse` checkout) |
| `streamclone-stack` | Health, ports, playback, auth, scraper, **compose_logs** (read-only logs) |
| `streamclone-data` | Read-only Postgres/Redis, emote jobs (local compose) |
| `streamclone-hosted-data` | Hosted prod PG/Redis (SSH tunnel; optional) |
| **Playwright** | UI smoke — `make smoke-ui`, `frontend/tests/playwright/` |

Setup: `make mcp-setup` then copy **`.cursor/mcp.windows.json.example`** → `~/.cursor/mcp.json` on Windows (or `mcp.recommended.json.example` on Linux/WSL). Preflight: `bash scripts/mcp-preflight.sh` (or `scripts/mcp-preflight.ps1` on Windows → WSL). Codex: `make codex-setup`.

---

## Docker Compose profiles

Detail: [docs/ENVIRONMENT.md](docs/ENVIRONMENT.md), [docs/SERVICE_MAP.md](docs/SERVICE_MAP.md).

| Profile | Adds | Typical use |
|---------|------|-------------|
| *(default core)* | metadata, video, chat, analytics, emote, frontend, local-proxy, postgres, redis, minio, mediamtx | Daily dev |
| `scraper` | TwitchTracker scraper (`:8000`) | Analytics minute charts |
| `clipper` | *(deprecated — maps to core)* | ReplayForge runs on host `:8095`; not a compose profile |

Env overlays: `deploy/env/profile-{core,scraper,full}.env`. Feature profiles are merged from `.env` via `scripts/lib/env.sh`.

---

## Code graph

Deep reference: [docs/agent-codegraph.md](docs/agent-codegraph.md) (`make codegraph-smoke` after rebuild).

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
- Go services: `cmd/*`, `internal/*` (social helpers: `internal/social/`)
- Frontend: `frontend/src/` (directory UX: `components/directory/`, `hooks/useMinSkeletonTime.ts`)
- Clipper (legacy stub): `clipper/` — active Clip Studio in sibling **ReplayForge** (`../replayforge`); see [docs/workspace.md](docs/workspace.md)
- Pulse extension spec: sibling **streamclone-pulse** (`../streamclone-pulse`); BFF and `packages/pulse-core/` in this repo
- Compose: `deploy/docker-compose*.yml`
- Agent steering: `.kiro/steering/`
