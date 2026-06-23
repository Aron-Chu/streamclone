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
| `.cursor/rules/*.mdc` | `.codex/rules/*.rules` (Starlark) | `.codex/rules/streamclone.rules` + user `~/.codex/rules/` |
| `AGENTS.md` | `AGENTS.md` (same file) | repo root |
| Cursor plugins (Figma cloud MCP) | Cursor plugin only | rate-limited; use **figma-bridge** or committed PNGs |
| Pulse extension UI design | `streamclone-pulse/docs/pulse-extension/figma-handoff.md` + `figma/*.png` | Codex reads images from repo — no Figma MCP required |

**Skills source of truth:** `.cursor/skills/streamclone/`. After editing skills in Cursor, run `make codex-sync-skills` (or `make codex-setup`).

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
