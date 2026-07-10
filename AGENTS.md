# Agent Guide

Product name: **Streamclone** (GitHub: `Aron-Chu/streamclone`). The local checkout folder may still be named `twitch-7tv-clone`; the release install lives at `%USERPROFILE%\streamclone` and is usually **not** this git repo.

**Product scope:** self-hosted Twitch replica — directory, HLS playback, chat, emotes, desktop install. **Not** StreamPulse backend, portal, extension BFF, or hosted ops (see [docs/streampulse-product-boundary.md](docs/streampulse-product-boundary.md)).

**Deep references:** [Service map](docs/SERVICE_MAP.md) · [Testing](docs/TESTING.md) · [MCP](docs/MCP.md) · [Codex](docs/CODEX.md) · [Environment](docs/ENVIRONMENT.md) · [Workspace layout](docs/workspace.md)

## Cursor vs generic agents

| Layer | Path | Role |
|-------|------|------|
| **This file** | `AGENTS.md` | Repo router — core product only |
| **Cursor rules** | `.cursor/rules/` | Cursor-specific behavior (`streamclone.mdc`, `agents-router.mdc`, domain rules) |
| **Cursor skills** | `.cursor/skills/streamclone/` | Watch / playback / emotes / install skills |
| **Cursor hooks** | `.cursor/hooks.json` | Lightweight gofmt/compose/codegraph hints |

StreamPulse extension and portal: sibling **streamclone-pulse** checkout and its `AGENTS.md`. Backend BFF and ingest: private **streampulse-backend**. Deploy and SSH: private **streampulse-ops**.

**Cross-repo index:** [`../streampulse-sdlc/AGENTS.md`](../streampulse-sdlc/AGENTS.md) · workspace: [`../streampulse-sdlc/streampulse-sdlc.code-workspace`](../streampulse-sdlc/streampulse-sdlc.code-workspace).

---

## Product scope (2026-07)

| In this repo | Elsewhere |
|--------------|-----------|
| Watch UI, directory UX, playback, chat, emotes | StreamPulse extension + portal → **streamclone-pulse** |
| Core Go services (`metadata`, `video`, `chat`, `emote`) | Analytics BFF, ingest, hub → **streampulse-backend** (private) |
| Core compose + desktop release CI | Production deploy, secrets, evidence → **streampulse-ops** (private) |

**Step 7 complete.** Do **not** re-add Analytics API, portal, ReplayForge/Clip Studio UI, or `cmd/analytics` / `internal/analytics` / `packages/analytics-console` here. Route those products to **streampulse-backend**, **streamclone-pulse**, or sibling **replayforge**.

**Verify after doc/code drift:** `make compose-config-check`, `cd frontend && npx tsc -b && npm test`, `curl http://127.0.0.1:8090/v1/metadata/health` or `make smoke`.

---

## Golden rules

1. **Preserve unrelated dirty work** — narrow diffs only; no drive-by refactors.
2. **No product behavior changes** unless the task explicitly asks for them.
3. **Read one steering doc** (see Task router) before broad file reads.
4. **Use the code graph MCP** before grepping the whole tree (`get_ast_chunk`, `get_blast_radius`).
5. **Route through Caddy** — browser and probes use `http://localhost:8090`, not raw service ports.
6. **Summarize API payloads** — do not paste full JSON into chat.
7. **Never commit secrets or production topology** — `.env`, tokens, `oauth-bundle.env`, machine-specific `.cursor/mcp.json`; no host IPs, SSH fingerprints, VPS paths, or operator runbooks in this public tree. See [`docs/security.md`](docs/security.md) and [docs/streampulse-product-boundary.md](docs/streampulse-product-boundary.md).
8. **Never edit applied migrations** — add new migration files only.
9. **Run narrow tests first**, then `make check-quick` or `make check` before a PR.
10. **Update steering/docs** after install, OAuth, playback, or large frontend changes; run `make codegraph` when symbols move.
11. **Three-repo split** — watch/install here; StreamPulse UI in **streamclone-pulse**; backend + ops private. Pick smallest workspace from [`docs/workspace.md`](docs/workspace.md).

## Agent context workflow (required)

1. Read exactly one domain steering/product doc from the task router.
2. Use codegraph MCP first (`get_ast_chunk`, `get_blast_radius`, `get_call_chain`).
3. Use stack/data MCP (`stack_health`, `stack_ports`, `compose_logs`, `postgres_query` SELECT only) when runtime truth matters.
4. Use `make context-snapshots` for route/schema summaries (core scope).
5. Use targeted file reads/grep only after MCP + snapshots are exhausted.

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
make check           # full gate (adds security-scan, audit, build)
make mcp-setup       # codegraph + mcp preflight (see docs/MCP.md)
make codex-setup     # Codex: .codex/config.toml + .agents/skills sync (see docs/CODEX.md)
bash scripts/mcp-preflight.sh
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
| **Emotes / 7TV / FFZ** | `.kiro/steering/emote-pipeline.md` | `emote_jobs`, `redis_channel_emotes` |
| **Local Twitch auth** | `.kiro/steering/local-auth.md` | `twitch_auth_status`, `make twitch-debug` |
| **Install / desktop / bootstrap** | `docs/install-desktop.md`, `docs/repo-maintenance.md` | Launcher scripts under `scripts/` |
| **Release / CI / images** | `.github/workflows/release-images.yml`, `VERSION`, `docs/repo-maintenance.md` | `make compose-config-check` |
| Clip Studio / ReplayForge | [docs/streampulse-product-boundary.md](docs/streampulse-product-boundary.md) | Sibling **replayforge** only — not this product |
| System health | `.kiro/steering/windows-dev.md` | `stack_health`, `get_ast_chunk("SystemHealthPanel")` |
| Security / secrets | `SECURITY.md`, `docs/security.md` | `make security-scan` |
| **StreamPulse (extension, portal, BFF, ingest, hosted ops)** | [docs/streampulse-product-boundary.md](docs/streampulse-product-boundary.md) | **streamclone-pulse**, **streampulse-backend**, or **streampulse-ops** — not this repo |

---

## Skills router

Load skills from `.cursor/skills/streamclone/` when the task fits. Codex mirrors: `.agents/skills/streamclone/` via `make codex-sync-skills`.

| Task | Skill |
|------|-------|
| Agent docs / Makefile / MCP setup | `agent-readiness/SKILL.md` |
| Claude Code setup | [`docs/CLAUDE.md`](docs/CLAUDE.md) — `make claude-setup` |
| Playback / HLS / latency | `playback-hls/SKILL.md` |
| Stack health / ports / localhost | `stack-debug/SKILL.md` |
| Emotes / 7TV / FFZ | `emote-pipeline/SKILL.md` |
| Windows install / Setup.exe / release bundle | `release-windows/SKILL.md` |
| Choosing tests by domain | `test-by-domain/SKILL.md` |
| Context ladder (codegraph → snapshots → Repomix) | `context-retrieval/SKILL.md` |
| StreamPulse extension / portal / backend / coverage | **streamclone-pulse** or private backend/ops checkouts — see boundary doc |

---

## Safe commands

| Command | Purpose |
|---------|---------|
| `make up` / `make stop` / `make ps` | Stack lifecycle |
| `make test` / `make vet` | Go unit tests (core packages) |
| `make test-video` / `test-emote` / `test-metadata` | Domain-scoped Go tests |
| `make frontend-test` / `make frontend-build` | Frontend checks |
| `make compose-config-check` | Validate compose merges |
| `make check-quick` | Fast combined gate |
| `make smoke` / `make smoke-ui` / `make agent-smoke` | Runtime smoke (needs stack) |
| `make validate-env PROFILE=core` | Env profile sanity |
| `make codegraph` / `make mcp-setup` | Rebuild AST graph / full MCP setup |
| `make context-snapshots` | Write `runtime/context/*.txt` summaries |
| `make context-verify` | Check rules, hooks, codegraph freshness |
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
| Applying optional migrations without task scope | Schema changes |

---

## Risky files and services

| Area | Paths / services | Notes |
|------|------------------|-------|
| Secrets | `.env*`, `runtime/`, `oauth-bundle.env` | Never commit |
| Migrations | `migrations/` | Forward-only |
| Compose / proxy | `deploy/docker-compose*.yml`, `deploy/Caddyfile*` | Wrong route breaks APIs |
| Playback | `deploy/mediamtx.yml`, `internal/video/`, `frontend/src/playback.ts` | HLS 401 if CDN secret mismatch |
| Auth | `internal/chat/auth*`, Twitch OAuth env | Session cookies, token import |
| Release | `VERSION`, `.github/workflows/release-images.yml` | Tag push triggers GHCR + Setup.exe |
| Agent config | `.cursor/mcp.json` | Gitignored; use `*.example` only |
| Removed products (do not re-add) | `cmd/analytics`, `internal/analytics/`, `packages/analytics-console`, Clip Studio UI | Route to streampulse-backend / streamclone-pulse / replayforge |

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
| Playwright UI | `make smoke-ui`, `frontend/tests/playwright/` |

---

## MCP (summary)

Full detail: [docs/MCP.md](docs/MCP.md).

| Server | Use for |
|--------|---------|
| `streamclone-codegraph` | Symbol lookup, call chains, blast radius (this repo) |
| `streamclone-stack` | Health, ports, playback, auth, **compose_logs** |
| `streamclone-data` | Read-only Postgres/Redis, emote jobs (local compose) |
| **Playwright** | UI smoke |

Hosted-data MCP and pulse codegraph belong in private backend / **streamclone-pulse** workspaces — not configured from this repo's default agent surface.

Setup: `make mcp-setup` then copy `.cursor/mcp.recommended.json.example` → `.cursor/mcp.json`.

---

## Docker Compose profiles

Detail: [docs/ENVIRONMENT.md](docs/ENVIRONMENT.md), [docs/SERVICE_MAP.md](docs/SERVICE_MAP.md).

| Profile | Typical use |
|---------|-------------|
| **core** (default) | metadata, video, chat, emote, frontend, local-proxy, postgres, redis, minio, mediamtx |

Env overlays: `deploy/env/profile-core.env`. Feature profiles merge from `.env` via `scripts/lib/env.sh`.

---

## Code graph

Deep reference: [docs/agent-codegraph.md](docs/agent-codegraph.md).

```sh
make codegraph-install
make codegraph
```

Database: `.codegraph/streamclone.kuzu` (gitignored).

---

## Layout

- User docs: `README.md`, `docs/`
- Go services: `cmd/metadata`, `cmd/video`, `cmd/chat`, `cmd/emote`, `cmd/healthcheck`
- Frontend: `frontend/src/` (directory, channel watch, playback, chat, emotes)
- Compose: `deploy/docker-compose*.yml`
- Agent steering: `.kiro/steering/`
