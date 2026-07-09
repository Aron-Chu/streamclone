# MCP guide for agents

Streamclone ships **read-only, repo-local** MCP servers for code navigation, stack diagnostics, and database inspection. Enable them in Cursor **Settings → MCP**; never commit your local `.cursor/mcp.json`.

**Scope:** core watch stack only. Hosted-data MCP, pulse codegraph, and backend BFF probes belong in **streampulse-backend** or **streamclone-pulse** workspaces. See [streampulse-product-boundary.md](streampulse-product-boundary.md).

**Philosophy:** read-heavy by default; write only through normal git edits; DB read-only; Docker access is logs/health/probes first.

---

## Priority matrix

| Priority | Tool | Why |
|----------|------|-----|
| **Essential** | `streamclone-codegraph` | Navigate Go/React symbols, blast radius, call chains |
| **Essential** | `streamclone-stack` | Docker health, ports, playback probes, bounded logs |
| **Essential** | `streamclone-data` | Read-only Postgres/Redis inspection (local compose) |
| **Essential** | Playwright MCP | Verify UI, diagnostics, playback states |
| **Essential** | GitHub (Cursor plugin or GitHub MCP) | PRs, issues, CI, diffs — prefer read-only tool scope |
| **Later** | Official docs lookup | Streamlink, MediaMTX, hls.js, ffmpeg, Twitch API |

### Avoid

- Full **write-access Docker** MCP — use `compose_logs` / `stack_health` instead
- Broad **filesystem** MCP when codegraph covers navigation
- Browser MCP **logged into personal accounts** by default
- MCP with **secret write** access unless heavily scoped
- **Hosted production** DB MCP from this repo's default config — use private ops checkout

---

## In-repo MCP servers

| Server | Script | Purpose |
|--------|--------|---------|
| **streamclone-codegraph** | `scripts/codegraph-mcp.sh` | AST/symbol graph over Go + TS (core repo) |
| **streamclone-stack** | `scripts/stack-mcp.sh` | Compose health, ports, playback, auth, **logs** |
| **streamclone-data** | `scripts/data-mcp.sh` | Read-only Postgres/Redis, emote jobs |

Extension/portal TS graph: configure **streamclone-pulse-codegraph** from the pulse checkout when working on extension or portal code.

---

## Setup

1. **Build graph and preflight**

   ```sh
   make mcp-setup
   ```

2. **Copy recommended config — do not commit your local file**

   - All platforms: [`.cursor/mcp.recommended.json.example`](../.cursor/mcp.recommended.json.example)
   - Linux / WSL: [`.cursor/mcp.linux.json.example`](../.cursor/mcp.linux.json.example)
   - Windows → WSL: [`.cursor/mcp.windows.json.example`](../.cursor/mcp.windows.json.example)

   **Codex:** run `make codex-setup` (writes `.codex/config.toml`, syncs skills). See [`docs/CODEX.md`](CODEX.md).

3. **Preflight**

   ```sh
   bash scripts/mcp-preflight.sh
   ```

   Windows: `.\scripts\mcp-preflight.ps1`

4. **Reload Cursor** — confirm servers show green with tools listed.

### Playwright (UI verification)

```sh
make smoke-ui
make agent-smoke    # stack up
```

Playwright tests: `frontend/tests/playwright/`.

---

## CI codegraph artifact

When `internal/`, `frontend/src/`, or `cmd/` change, CI rebuilds the graph and uploads `.codegraph/streamclone.kuzu` (7-day retention). Local `make codegraph` is preferred for day-to-day work.

---

## Windows vs WSL

- Wrapper scripts (`scripts/*-mcp.ps1`) launch **WSL bash**.
- Port **8090** owned by `wslrelay` → `.kiro/steering/windows-dev.md`; `stack_ports` warns.
- Do **not** commit absolute Windows paths — use `${workspaceFolder}`.

---

## Warnings

- **Do not commit** `.cursor/mcp.json`, `.env`, tokens, or API keys in MCP env blocks.
- `postgres_query` is read-only by design.
- `rebuild_graph` can be CPU-heavy.

---

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| Codegraph tools empty | `make codegraph`; check `graph_status()` |
| Preflight fails on `kuzu` / `mcp` | `make mcp-setup` |
| Stack tools 401/connection refused | `make up`; check `stack_ports` |
| Data tools DB error | Postgres container up? `DATABASE_URL` in `.env` |
| Handshake timeout | `scripts/verify-mcp-stdio.ps1` or preflight stderr |

List tools: `bash scripts/mcp-list-tools.sh`.

Extension/portal codegraph: **streamclone-pulse** checkout (separate graph if configured).
