# Codex setup (Streamclone)

Mirror Cursor agent tooling — MCP, skills, rules — for [OpenAI Codex](https://developers.openai.com/codex/) CLI / IDE extension.

Cursor uses `.cursor/mcp.json` (gitignored) and `.cursor/skills/`. Codex uses **`.codex/config.toml`**, **`.agents/skills/`**, and **`.codex/rules/`**. This repo ships project-layer files for **core watch product** tooling only.

StreamPulse backend skills and hosted MCP → private **streampulse-backend** or **streamclone-pulse**. See [streampulse-product-boundary.md](streampulse-product-boundary.md).

---

## Quick start (Windows)

```powershell
cd C:\Users\Aron\twitch-7tv-clone   # your streamclone checkout
make codex-setup                    # writes .codex/config.toml + syncs skills
make mcp-setup                      # codegraph DB + MCP preflight (WSL)
```

In Codex:

1. **Trust this repository** (required for project `.codex/` to load).
2. Reload Codex or run **`/mcp`** — expect `streamclone-codegraph`, `streamclone-stack`, `streamclone-data`, `playwright`.
3. Open the repo — Codex reads root **`AGENTS.md`** plus **`.codex/AGENTS.md`**.

For extension/portal work, open **streamclone-pulse** or its multi-root workspace — not this repo alone.

---

## What maps from Cursor → Codex

| Cursor | Codex | Streamclone location |
|--------|-------|----------------------|
| `.cursor/mcp.json` | `[mcp_servers.*]` in config | `.codex/config.toml` (generated) |
| `.cursor/skills/streamclone/` | `.agents/skills/streamclone/` | synced by `make codex-setup` |
| `.cursor/rules/*.mdc` | `.codex/rules/*.rules` (Starlark) | `.codex/rules/streamclone.rules` |
| `AGENTS.md` | `AGENTS.md` (same file) | repo root |

**Skills source of truth:** `.cursor/skills/streamclone/` only. After editing skills, run `make codex-sync-skills` (or `make codex-setup`).

Pulse/backend skills are **not** synced from this repo after the boundary split.

**MCP details:** same core servers as [`docs/MCP.md`](MCP.md) — codegraph, stack, data, Playwright.

---

## Files in this repo

```text
.codex/
  config.toml.example    # template (__REPO_WIN__ placeholders)
  config.toml            # generated — gitignored
  AGENTS.md              # Codex-only notes
  rules/streamclone.rules
.agents/skills/streamclone/   # mirrored skills (Codex discovery)
docs/codex/global-config.toml.example
scripts/codex-setup.ps1
scripts/codex-sync-skills.sh
scripts/codex-mcp-launch.sh
```

---

## Global Codex home (`~/.codex/`)

Optional personal layer — merge [`docs/codex/global-config.toml.example`](codex/global-config.toml.example) into `~/.codex/config.toml`.

---

## Linux / WSL-native Codex

If Codex runs inside WSL (not `wsl.exe` from Windows), replace MCP entries with bash directly — see `.codex/config.toml.example`.

---

## Verify

```powershell
make codex-setup
bash scripts/mcp-preflight.sh
```

In Codex session: `/mcp` — list tools.

---

## Copy-paste prompt for Codex

```text
Read AGENTS.md and .codex/AGENTS.md. Load the matching skill from .agents/skills/streamclone/ before broad file reads.
Use streamclone-codegraph (get_ast_chunk, get_blast_radius) before repo-wide grep.
Stack probes via streamclone-stack at http://localhost:8090.
DB read-only via streamclone-data.
Follow .codex/rules/ and commits policy in AGENTS.md (Aron-Chu author, Conventional Commits).
```

For StreamPulse / BFF / ingest / portal work, switch to **streamclone-pulse** or private **streampulse-backend** checkout.

---

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| No project MCP | Trust repo; run `make codex-setup`; reload Codex |
| Codegraph empty | `make codegraph`; `graph_status()` |
| `wsl.exe` fails | WSL installed; path in `.codex/config.toml` matches checkout |
| Skills missing | `make codex-sync-skills`; restart Codex |
