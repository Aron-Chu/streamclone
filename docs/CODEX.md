# Codex setup (Streamclone)

Mirror Cursor agent tooling — MCP, skills, rules — for [OpenAI Codex](https://developers.openai.com/codex/) CLI / IDE extension.

Cursor uses `.cursor/mcp.json` (gitignored) and `.cursor/skills/`. Codex uses **`.codex/config.toml`**, **`.agents/skills/`**, and **`.codex/rules/`**. This repo ships project-layer files so both agents share the same Streamclone probes and skills.

---

## Quick start (Windows)

```powershell
cd C:\Users\Aron\twitch-7tv-clone   # your streamclone checkout
make codex-setup                    # writes .codex/config.toml + syncs skills
make mcp-setup                      # codegraph DB + MCP preflight (WSL)
```

In Codex:

1. **Trust this repository** (required for project `.codex/` to load). If MCP tools are missing, add the repo path under trusted projects in Codex settings or `~/.codex/config.toml`.
2. Reload Codex or run **`/mcp`** — expect `streamclone-codegraph`, `streamclone-stack`, `streamclone-data`, `playwright`.
3. Open the repo — Codex reads root **`AGENTS.md`** plus **`.codex/AGENTS.md`**.

Optional multi-root (extension + main): open `streamclone-pulse-extension.code-workspace` in Cursor. Extension spec: `streamclone-pulse/docs/pulse-extension/`; backend + MCP in **streamclone** folder.

---

## What maps from Cursor → Codex

| Cursor | Codex | Streamclone location |
|--------|-------|----------------------|
| `.cursor/mcp.json` | `[mcp_servers.*]` in config | `.codex/config.toml` (generated) |
| `.cursor/skills/streamclone/` | `.agents/skills/streamclone/` | synced by `make codex-setup` |
| `.cursor/skills/pulse/` | `.agents/skills/pulse/` | synced by `make codex-sync-skills` (Pulse coverage/backfill review) |
| `.cursor/rules/*.mdc` | `.codex/rules/*.rules` (Starlark) | `.codex/rules/streamclone.rules` + user `~/.codex/rules/` |
| `AGENTS.md` | `AGENTS.md` (same file) | repo root |
| Cursor plugins (Figma cloud MCP) | Cursor plugin only | rate-limited; use **figma-bridge** or committed PNGs |
| Pulse extension UI design | `streamclone-pulse/docs/pulse-extension/figma-handoff.md` + `figma/*.png` | Codex reads images from repo — no Figma MCP required |

**Skills source of truth:** `.cursor/skills/streamclone/` and `.cursor/skills/pulse/`. After editing skills in Cursor, run `make codex-sync-skills` (or `make codex-setup`).

**MCP details:** same servers as [`docs/MCP.md`](MCP.md) — codegraph, stack, data, Playwright.

---

## Files in this repo

```text
.codex/
  config.toml.example    # template (__REPO_WIN__ placeholders)
  config.toml            # generated — gitignored, machine paths
  AGENTS.md              # Codex-only notes (supplements root AGENTS.md)
  rules/streamclone.rules
.agents/skills/streamclone/   # mirrored skills (Codex discovery)
.agents/skills/pulse/         # Pulse live coverage / backfill / capacity review skills
docs/codex/global-config.toml.example   # optional merge into ~/.codex/
scripts/codex-setup.ps1
scripts/codex-sync-skills.sh
scripts/codex-mcp-launch.sh
```

---

## Global Codex home (`~/.codex/`)

Optional personal layer — merge [`docs/codex/global-config.toml.example`](codex/global-config.toml.example) into `~/.codex/config.toml`:

- Commit author rules (`developer_instructions`)
- Extra MCP you use everywhere (GitHub read-only, etc.)
- `project_doc_fallback_filenames` if you use alternate doc names

Create `~/.codex/AGENTS.md` for personal working agreements (Codex reads it for every repo).

---

## Linux / WSL-native Codex

If Codex runs inside WSL (not `wsl.exe` from Windows), replace MCP entries with bash directly:

```toml
[mcp_servers.streamclone-codegraph]
command = "bash"
args = ["scripts/codex-mcp-launch.sh", "codegraph"]
```

Use `.cursor/mcp.recommended.json.example` as the parallel for Linux paths.

---

## Verify

```powershell
make codex-setup
bash scripts/mcp-preflight.sh    # or scripts/mcp-preflight.ps1 on Windows
```

In Codex session:

```text
/mcp
```

```text
Summarize the current instructions.
```

Expected: global + project `AGENTS.md`, skills under `.agents/skills/streamclone/`, MCP tools listed.

---

## Config review (before big prompts)

**Project layer:** `.codex/config.toml` (generated from `config.toml.example`).

| Setting | Recommendation |
|---------|----------------|
| **Trust** | `trust_level = "trusted"` for streamclone + streamclone-pulse in `~/.codex/config.toml` — you have this |
| **Full access vs sandbox** | **Trusted + rules** beats unrestricted: allows MCP + probes; `streamclone.rules` still blocks `make nuke`, `git push --force` |
| **`project_doc_max_bytes`** | **131072** (128 KiB) — portal PRD alone is ~62 KiB |
| **Essential MCP** | codegraph, stack, data — keep enabled |
| **playwright / figma-bridge** | **Disable in project config** for architecture reviews (enable in UI when needed); avoid duplicating global `figma-bridge` |
| **Duplicate figma-bridge** | If `~/.codex/config.toml` already defines figma-bridge, keep project `enabled = false` |

Regenerate after example edits: `make codex-setup`.

---

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| No project MCP | Trust repo; run `make codex-setup`; reload Codex |
| Codegraph empty | `make codegraph`; `graph_status()` |
| `wsl.exe` fails | WSL installed; path in `.codex/config.toml` matches checkout |
| Skills missing | `make codex-sync-skills`; restart Codex |
| Wrong `mcp_servers` key | Codex uses `mcp_servers` not `mcpServers` (TOML) |

---

## Exact prompt for Codex (copy-paste)

```text
Read AGENTS.md and .codex/AGENTS.md. Load the matching skill from .agents/skills/streamclone/ before broad file reads.
Use streamclone-codegraph (get_ast_chunk, get_blast_radius) before repo-wide grep.
Stack probes via streamclone-stack at http://localhost:8090.
DB read-only via streamclone-data.
Follow .codex/rules/ and commits policy in AGENTS.md (Aron-Chu author, Conventional Commits).
```

For analytics work:

```text
Read .agents/skills/streamclone/analytics-sync/SKILL.md and .kiro/steering/analytics.md.
Use get_blast_radius before editing sync/rollup code.
```

For Pulse extension UI (Codex without Figma cloud MCP):

```text
Read streamclone-pulse/docs/pulse-extension/figma-handoff.md and open PNGs in docs/pulse-extension/figma/.
Match design tokens in the handoff; requirements in docs/pulse-extension/requirements.md.
```

---

## Pulse live coverage architecture review (Codex)

Use this for smart-model reviews of **live tracking from stream start**, **VOD chat backfill**, **Protect channel**, and **BearHost** capacity — without implementing code unless asked.

### Setup checklist

```powershell
cd C:\Users\Aron\twitch-7tv-clone
make codex-setup          # .codex/config.toml + sync .agents/skills/*
make mcp-setup            # codegraph DB + preflight
make codegraph            # if graph_status() is stale
```

In Codex:

1. **Trust this repository** (required for `.codex/config.toml`).
2. Open **`streamclone-pulse-extension.code-workspace`** (both repos) or ensure `../streamclone-pulse` exists.
3. Run **`/mcp`** — expect `streamclone-codegraph`, `streamclone-stack`, `streamclone-data` (playwright optional).
4. Load skill **`pulse-live-coverage-review`** (`.agents/skills/pulse/`).

### Skills to load

| Skill | Path |
|-------|------|
| **pulse-live-coverage-review** | `.agents/skills/pulse/pulse-live-coverage-review/SKILL.md` |
| coverage-triage | `.agents/skills/pulse/coverage-triage/` |
| backfill-safety-review | `.agents/skills/pulse/backfill-safety-review/` |
| capacity-governor-review | `.agents/skills/pulse/capacity-governor-review/` |
| api-contract-drift-check | `.agents/skills/pulse/api-contract-drift-check/` |
| analytics-sync | `.agents/skills/streamclone/analytics-sync/` |
| context-retrieval | `.agents/skills/streamclone/context-retrieval/` |

After editing skills: `make codex-sync-skills`.

### MCP policy for reviews

| Use | Skip |
|-----|------|
| `get_ast_chunk`, `get_blast_radius`, `get_call_chain` | Playwright (unless UI task) |
| `stack_health`, `compose_logs(analytics)` | figma-bridge |
| `postgres_query` (SELECT), `redis_get` | Write Docker MCP |
| `curl` localhost:8090 and hosted `/v1/extension/health` | Browser on twitch.tv |

**Codegraph seed symbols:** `computePulseCoverage`, `SyncPulseMissedChat`, `PulseBackfillManager`, `Collector`, `extensionPulseChannel`.

**Helper scripts:**

```bash
python .cursor/skills/pulse/backfill-safety-review/scripts/backfill-smoke.py --login CHANNEL
python .cursor/skills/pulse/api-contract-drift-check/scripts/contract-keys.py
```

### Copy-paste review prompt

```text
MODE: architecture review only — no code edits unless I ask.

Load skill pulse-live-coverage-review and companions: coverage-triage, backfill-safety-review, capacity-governor-review, analytics-sync, context-retrieval.

Read first:
- streamclone-pulse/docs/pulse-extension/live-coverage-requirements.md
- docs/roadmapping.md, docs/tools.md, docs/finalplan.md (Phases B–D)
- deploy/env/profile-bearhost-pulse.env

Use streamclone-codegraph before broad grep. MCP stack_health + postgres_query SELECT only.
curl http://localhost:8090/v1/extension/health and https://api.streampulse.stream/v1/extension/health (no secrets in output).

Mission: Pulse tracks live from when tracking begins. Late join → VOD backfill only when Twitch replay exists. Protect channel → future streams from ~00:00.

Review:
1. Reality check — is ~60% built (gap = deploy + go-live worker + VOD finalization) accurate?
2. Optimal 4–8 week build sequence
3. Algorithm improvements (VOD fetch, rollup merge, peak recompute, go-live scheduling, cap preemption)
4. Architecture foot-guns (dual backfill pipelines, EventSub placement, BFF 12s cache, extension GQL hints)
5. Capacity model for BearHost 8GB (PULSE_MAX_ACTIVE_CHANNELS=10 today)
6. Doc gaps and overpromises
7. Top 10 P0/P1 tickets

Output: executive summary → build sequence table → ranked algorithm proposals → risks → tickets.

Be direct. Disagree with our assumptions where warranted.
```

### Docs map

| Doc | Role |
|-----|------|
| [`streamclone-pulse/docs/pulse-extension/live-coverage-requirements.md`](../streamclone-pulse/docs/pulse-extension/live-coverage-requirements.md) | Canonical requirements |
| [`docs/pulse-extension/live-coverage-requirements.md`](pulse-extension/live-coverage-requirements.md) | Redirect stub (this repo) |
| [`docs/roadmapping.md`](roadmapping.md) | Phased delivery R0–R6 |
| [`docs/tools.md`](tools.md) | Data source tiers |
| [`docs/finalplan.md`](finalplan.md) | Archive / session architecture |
