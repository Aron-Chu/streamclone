# Agent context stack

How Streamclone agents pick context without burning tokens. Complements [`AGENTS.md`](../AGENTS.md) and [`docs/MCP.md`](MCP.md).

## Layers (cheap → expensive)

| Layer | Tool | Use for | Avoid for |
|-------|------|---------|-----------|
| Router | `AGENTS.md` + task table | Where to look first | Symbol-level lookup |
| Rules | `.cursor/rules/*.mdc` | Domain guardrails when matching files are open | Runtime DB state |
| Code graph | MCP `streamclone-codegraph` | Symbols, callers, blast radius | Live queue depths |
| Runtime | MCP stack/data **or** `make context-snapshots` | Health, routes, schema, backfill rows | Finding a function body |
| Repomix | `repomix.config.json` | Rare full-tree questions | Every task |
| CocoIndex | Optional local MCP | Semantic "where is X discussed" | Exact TS/Go defs (use codegraph) |

## Token budget (practical)

**Always-on cost (every Cursor turn in this repo):**

- `agents-router.mdc`, `streamclone-naming.mdc`, `commits.mdc` — ~400–600 tokens combined
- Workspace/user rules outside the repo — varies

**Usually free until triggered:**

- Glob rules (`backend-go`, `frontend-react`, `analytics`, `playback`, …) — only when relevant files are in context
- Skills — only when invoked or strongly matched
- Subagents — separate context window
- Snapshots / Repomix — only if agent runs script and reads output

**Expensive mistakes:**

- `alwaysApply: true` on everything → cut this; pulse guardrails use globs in backend repo
- Pasting Repomix output or full pulse API JSON into chat
- Running `make check` on every file save (hooks intentionally avoid this)

## Verify it is working

### 1. Static stack (no Docker)

```bash
make context-verify
```

Expect `OK` for rules, hooks, codegraph DB. `MISS` on codegraph → `make mcp-setup`.

### 2. Rules firing

Open a Go file under `internal/analytics/` and ask:

> "Which steering doc applies to this file?"

Agent should cite `backend-go.mdc` / analytics steering, not generic advice.

### 3. Codegraph firing

Ask:

> "Who calls mergeMinuteRollups?"

Agent should use MCP `get_blast_radius` or `get_call_chain`, not repo-wide grep first.

### 4. Runtime snapshots

```bash
make up
make context-snapshots
```

Ask:

> "Summarize active backfill_jobs from the latest snapshot."

Agent should read `runtime/context/backfill_status.txt` (short), not query inventing data.

### 5. Hooks (Cursor)

Edit a `.go` file → gofmt runs (backend repo).
Run `git commit` with `PULSE_BETA_KEYS=realvalue` staged → hook should deny.

Check **Cursor → Output → Hooks** if a hook does not fire.

### Cursor reload

| Change | Reload needed |
|--------|----------------|
| `.cursor/rules/*.mdc` | Usually immediate; **Reload Window** if agent ignores new rule |
| `.cursor/hooks.json` | Watches on save; restart Cursor if hooks never appear |
| `.cursor/agents/*.md` | Reload Window once |
| MCP servers in `.cursor/mcp.json` | **Restart Cursor** or toggle MCP off/on |
| `AGENTS.md`, skills, scripts | No restart |

Full Cursor restart is rarely required; **Developer: Reload Window** is enough for most `.cursor/` edits.

## Codex CLI + Claude Code

| Tool | Role |
|------|------|
| **Codex CLI** | Same `AGENTS.md` + `.agents/skills` mirror (`make codex-sync-skills`) |
| **Cursor** | Rules, hooks, subagents, MCP in IDE |
| **Claude Code** | Optional for subagent-style review; keep prompts in `.cursor/agents/` as single source |

Do not maintain three different guardrail copies — product law lives in docs; routing in `AGENTS.md`; Cursor-only in `.cursor/`.

## Cursor Automations — when they help

Automations are **not** a substitute for rules/hooks. Useful cases:

| Automation | Trigger | Why |
|------------|---------|-----|
| Hosted health | Cron daily | `curl https://api.streampulse.stream/v1/extension/health` → notify if fail |
| PR hygiene | PR opened on pulse/backend | Run tests summary + `backend-safety-reviewer` subagent |
| tasks.md drift | Weekly | Remind if `docs/website-portal/tasks.md` checkboxes without linked commits |

Skip automations for: every save lint, codegraph rebuild, or normal feature work — hooks + MCP cover that locally.

Not worth it yet: automations that duplicate `make check-quick` on every push (CI already exists).

---

## Plugin + MCP audit (token-aware)

### In-repo MCP (configure in `.cursor/mcp.json` — essential)

| Server | Tools | Use when | Skip when | Token note |
|--------|-------|----------|-----------|------------|
| **streamclone-codegraph** | 12 | Symbol lookup, blast radius, call chain | Runtime health, DB rows | Returns **slices**, not whole files — cheap |
| **streamclone-stack** | 6 | Stack up: health, ports, HLS, logs | Stack down | Response can be large — ask for summary |
| **streamclone-data** | 5 | SELECT queries, emote jobs, Redis keys | Finding Go functions | Cap 200 rows — still summarize |
| **playwright** | many | UI smoke, playback states | Go-only backend edits | Heavy; enable only for frontend tasks |

**Minimal profile** (extension-only days): copy [`.cursor/mcp.minimal.json.example`](../.cursor/mcp.minimal.json.example) — codegraph + data only.

**Pulse extension checkout:** copy [streamclone-pulse `.cursor/mcp.recommended.json.example`](../../streamclone-pulse/.cursor/mcp.recommended.json.example) — points WSL at sibling backend.

Verify: `make context-verify` (includes MCP preflight). In Cursor: **Settings → MCP** — all streamclone servers green. If agents still grep first, reload window.

### Cursor plugins (installed in your environment)

| Plugin | MCP / skills | Use for Streamclone | Leave off / defer | Token risk |
|--------|--------------|---------------------|-------------------|------------|
| **Cloudflare** | 4 MCP servers + Workers skills | StreamPulse Pages, Tunnel, D1, Workers | Go analytics, extension overlay | Bindings MCP needs auth — don't load unless doing CF infra |
| **Figma** | figma MCP + skills | Pulse extension UI from `docs/pulse-extension/figma/` | Backend, scraper | Only open when implementing UI |
| **Grafana Assistant** | rules + skills | `pulse` profile dashboards | Default dev on core stack | **Always-on rule** — keep plugin disabled unless doing Grafana work |
| **SonarQube** | MCP (needs setup) + skills | Pre-PR / release quality gate | Daily iteration | Full analysis is expensive — use intentionally |
| **Modern Web Guidance** | Chrome extension skill | MV3 extension APIs | Go services | Skill triggers on extension HTML/CSS only |
| **cursor-ide-browser** | browser MCP | Playback/UI verification | Batch refactors | Screenshots cost context — use targeted |

### What this session actually had access to

Plugin MCPs (Figma, Cloudflare docs, SonarQube, browser) appear in the agent tool list when plugins are enabled. **streamclone-codegraph/stack/data only appear if** `.cursor/mcp.json` is configured **and** Cursor shows them green for the **streamclone** workspace root. Multi-root: open the backend folder as primary root or merge MCP config per folder.

If streamclone MCPs are missing, agents fall back to Grep/Read — **works but costs more tokens** and misses blast radius.

### Effective stack (recommended)

```text
Every turn (small):     agents-router + naming + commits  (~500 tokens)
File open (free until): backend-go / frontend-react / analytics / playback globs
On demand:              codegraph MCP → stack/data MCP OR context-snapshots
Rare:                   Repomix, subagent reviewers, Playwright
Plugins:                enable per task domain — not all at once
```

### Red flags ( wasting tokens )

- Grafana or SonarQube plugin enabled during routine Pulse backend work
- Repomix or full `postgres_query` dumps pasted into chat
- `alwaysApply: true` on pulse guardrails in both repos (pulse repo now glob-only)
- CocoIndex + codegraph + Repomix for the same question
- Playwright MCP enabled when not doing UI verification
