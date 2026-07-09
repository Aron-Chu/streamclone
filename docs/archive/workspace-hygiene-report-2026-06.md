# Workspace hygiene report

**Date:** 2026-06-21
**Repos:** [Aron-Chu/streamclone](https://github.com/Aron/streamclone), [Aron-Chu/streamclone-pulse](https://github.com/Aron/streamclone-pulse)
**Scope:** Phase 1 implementation (no commits)

---

## Summary

Runtime code is clean — no stale `console.log`, `debugger`, or agent debug ingest. Hygiene work focused on doc/router alignment for the Pulse extension, dead re-export shims, clipper compose profile removal, redirect stub fixes, and agent navigation refresh.

---

## Changes by area

### Code / config (streamclone)

| Item | Action |
|------|--------|
| Root `grafana-archive-*.png` | Deleted; `/grafana-archive-*.png` added to `.gitignore` |
| `frontend/src/utils/{liveHeat,momentScore,momentScoring,vodDeepLink}.ts` | Deleted (one-line `@streamclone/pulse-core` re-exports; zero imports) |
| `Makefile` `COMPOSE_FULL` | Removed `--profile clipper` |
| `refresh-clipper-token` | No longer recreates deprecated compose clipper service |
| `scripts/ensure-clipper-auth.ps1` | ReplayForge on host `:8095`; no `--profile clipper` |
| `.cursor/skills/streamclone/clipper-local/SKILL.md` | ReplayForge sibling boundary; synced to `.agents/skills/` |

### Code (streamclone-pulse)

| Item | Action |
|------|--------|
| `stopAllPolling` | Removed from `src/background/tracking.ts` (unused export) |

### Docs (streamclone)

| Item | Action |
|------|--------|
| `AGENTS.md` | Pulse extension task router row, skills cross-link, risky files, `docs/workspace.md` in deep refs |
| `docs/SERVICE_MAP.md` | Analytics routes include `/v1/extension/*`, `/v1/pulse/*` |
| `docs/workspace.md` | **New** — two-repo layout, doc ownership, dev workflow |
| `docs/repo-maintenance.md` | Index updated; pending install log rows closed or split |
| `.kiro/steering/{clipper,analytics,tech}.md` | Fixed broken relative links |
| `.codex/AGENTS.md` | Fixed links to root AGENTS and CODEX docs |
| `docs/pulse-extension/*` | **Removed** — canonical docs in streamclone-pulse only; see streampulse-product-boundary.md |

### Spec (streamclone-pulse)

| Item | Action |
|------|--------|
| `requirements.md` | Status → **MVP shipped / in progress**; code paths cite pulse-core + extension + BFF |
| `tasks.md` | P1-4 Redis BFF cache marked complete |
| `CONTEXT.md` | Codex (`.agents/skills`) vs Cursor (`.cursor/skills`) note |

---

## Tests

| Suite | Result |
|-------|--------|
| `streamclone-pulse`: `npm test` | 3 passed |
| `streamclone-pulse`: `npm run typecheck` | OK |
| `streamclone/frontend`: `npm run test` | 348 passed |

---

## Codegraph / MCP

```
make codegraph
→ 946 source files, 5692 definitions, 35778 resolved calls

bash scripts/mcp-preflight.sh → Preflight passed
  codegraph: 6 tools | stack: 6 tools | data: 5 tools
```

**MCP setup (local, not committed):** Copy [`.cursor/mcp.recommended.json.example`](../.cursor/mcp.recommended.json.example) → `.cursor/mcp.json`, run `make mcp-setup`, reload Cursor.

Parse warnings (non-fatal): `Channel.tsx`, SQL migrations, `pulse-core/src/index.ts`, `runtime/*-debug.sql`.

---

## Intentionally kept

- `clipper/` stub — still in `make clipper-test` / `make check`
- `packages/pulse-core`, extension BFF/bookmarks/recap
- Grafana tunnel/watch/bearhost scripts (Makefile + docs wired)
- Extension Figma PNGs — **streamclone-pulse** `docs/pulse-extension/figma/` only

---

## Still open (future hygiene)

- Migrate `frontend/src/utils/emoteImageUrl.ts` and `vodId.ts` to `@streamclone/pulse-core` (duplicate, not dead)
- Repo hygiene bucket in install log (CodeQL, CODEOWNERS, issue templates)
- Install automation partial items (Pulse Wire defaults, warming UI) — close per verified item
- Optional: drop streamclone-only Figma export script if multi-root workspace is always used

---

## Multi-root workspace

Open [`streamclone-pulse-extension.code-workspace`](../streamclone-pulse-extension.code-workspace). See [docs/workspace.md](workspace.md) for commit workflow and doc ownership.
