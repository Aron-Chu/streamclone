---
name: Hosted MCP + Gap Fixes
overview: Add a read-only hosted BearHost Postgres/Redis MCP server (via SSH-tunneled `app_readonly` role), a hosted-aware health tool, a second codegraph instance for the streamclone-pulse repo, and wire up `.cursor/mcp.json` in both repos with matching example/doc updates.
todos:
  - id: provision-readonly-role
    content: Write scripts/bearhost-provision-readonly-role.sh (app_readonly role, statement_timeout, connection limit) and run it against BearHost with confirmation
    status: completed
  - id: hosted-data-launcher
    content: Write scripts/hosted-data-mcp.sh (+ .ps1) with fail-closed port checks, SSH tunnel, cleanup trap, and secrets file convention
    status: completed
  - id: data-mcp-labeling
    content: Add --server-name/--label args to tools/data/data_mcp.py and surface target_label in data_status()
    status: completed
  - id: hosted-data-smoke
    content: Write opt-in scripts/hosted-data-mcp-smoke.sh (stdio handshake against the real tunnel), excluded from default preflight
    status: completed
  - id: hosted-health-tool
    content: Add hosted_health(mode) tool to tools/stack/stack_mcp.py
    status: completed
  - id: pulse-codegraph-launcher
    content: Write scripts/codegraph-pulse-mcp.sh (+ .ps1) and Makefile codegraph-pulse target for a second codegraph instance over streamclone-pulse
    status: completed
  - id: pulse-subsystems-seed
    content: Add optional streamclone-pulse/tools/codegraph/subsystems.json seed file
    status: completed
  - id: update-mcp-examples
    content: Update .cursor/mcp.*.example in both repos with the two new servers (skip mcp.minimal.json.example)
    status: completed
  - id: wire-local-mcp-json
    content: Create real, gitignored .cursor/mcp.json in both repos from the updated examples
    status: completed
  - id: preflight-updates
    content: Update scripts/mcp-preflight.sh and mcp-list-tools.sh for the new servers, keeping hosted-data opt-in only
    status: completed
  - id: docs-updates
    content: Fix docs/MCP.md tool-count drift, document new servers/tools and hosted-data security model, update docs/agent-codegraph.md and streamclone-pulse/docs/CONTEXT.md codegraph-gap sections
    status: completed
  - id: verify
    content: Run preflight + hosted smoke, reload Cursor, confirm all 5 servers green with tools in both workspace roots
    status: completed
isProject: false
---

# Improve Streamclone MCP tooling for hosted data + StreamPulse workspace

## 1. Hosted read-only Postgres/Redis MCP (`streamclone-hosted-data`)

**DB-side hardening (BearHost, one-time, operator-confirmed):**

New [`scripts/bearhost-provision-readonly-role.sh`](c:\Users\Aron\twitch-7tv-clone\scripts\bearhost-provision-readonly-role.sh) — idempotent, run over the existing `bearhost_ssh` helper ([`scripts/lib/bearhost-ssh.sh`](c:\Users\Aron\twitch-7tv-clone\scripts\lib\bearhost-ssh.sh)):

- `docker exec -i streamclone-postgres-1 psql -U app -d streamclone` with heredoc SQL that creates/updates `app_readonly`:
  - `LOGIN`, no superuser/createdb/createrole, `CONNECTION LIMIT 5`
  - `ALTER ROLE app_readonly SET statement_timeout = '5s'` (protects prod from runaway ad-hoc SELECTs — this is DB-side, independent of any client)
  - `GRANT CONNECT`, `GRANT USAGE ON SCHEMA public`, `GRANT SELECT ON ALL TABLES IN SCHEMA public`, plus `ALTER DEFAULT PRIVILEGES ... GRANT SELECT` so future tables stay covered
- Generates a password (`openssl rand -base64 24`) if `BEARHOST_READONLY_DB_PASSWORD` isn't already set, prints it once, and appends it to a new **local, gitignored, outside-repo** file `~/.streamclone/hosted-data-mcp.env` (mirrors the existing `~/.streamclone/` operator-secrets convention) — never written into the repo or `.cursor/mcp.json`.
- This is a manual/operator-run script against production — I will run it with your confirmation, not silently.

**Tunnel + launcher:**

New [`scripts/hosted-data-mcp.sh`](c:\Users\Aron\twitch-7tv-clone\scripts\hosted-data-mcp.sh) (WSL bash, same pattern family as `scripts/bearhost-grafana-tunnel.sh`):

1. Source `bearhost_ssh_config` + optional `~/.streamclone/hosted-data-mcp.env`.
2. Fail closed if `127.0.0.1:15432` or `127.0.0.1:16379` are already bound (stale tunnel or port conflict) — refuse to start rather than silently reuse an unknown listener.
3. Start `ssh -i $BEARHOST_SSH_KEY -N -o ExitOnForwardFailure=yes -o ServerAliveInterval=30 -o ServerAliveCountMax=3 -L 127.0.0.1:15432:127.0.0.1:5432 -L 127.0.0.1:16379:127.0.0.1:6379 $BEARHOST_USER@$BEARHOST_HOST` in the background; poll (≤10s) until both forwarded ports accept connections, or exit non-zero if the ssh process dies first (fail closed, no silent fallback to a broken tunnel).
4. **Run (not `exec`) foreground** `tools/data/data_mcp.py --postgres-url postgres://app_readonly:$BEARHOST_READONLY_DB_PASSWORD@127.0.0.1:15432/streamclone?sslmode=disable --redis-url redis://127.0.0.1:16379/0 --server-name streamclone-hosted-data --label "BearHost hosted (read-only, app_readonly)"` — kept in foreground (not `exec`'d) specifically so a `trap cleanup EXIT INT TERM` can kill the SSH tunnel child when Cursor stops the MCP subprocess. No public port is opened anywhere; the tunnel rides entirely inside the existing SSH session already used for all other BearHost ops.
5. New `scripts/hosted-data-mcp.ps1` — thin manual-launch wrapper mirroring `scripts/data-mcp.ps1`.

**Code change** — small addition to existing [`tools/data/data_mcp.py`](c:\Users\Aron\twitch-7tv-clone\tools\data\data_mcp.py):

```2:38:c:\Users\Aron\twitch-7tv-clone\tools\data\data_mcp.py
"""Read-only Postgres and Redis MCP tools for Streamclone local debugging."""
...
mcp = FastMCP(
    "streamclone-data",
    instructions=(
        "Read-only Postgres and Redis inspection for Streamclone emotes, analytics, and chat cache. "
        "Connects to localhost compose services (5432 / 6379)."
    ),
    log_level="ERROR",
)
```

Add `--server-name` (default `streamclone-data`) and `--label` (default `local dev stack (127.0.0.1)`) CLI args, use them in the `FastMCP(...)` instructions and add `"target_label"` to `data_status()`'s response — so the hosted instance shows up distinctly in Cursor's MCP panel and every tool result is self-describing about which environment it hit. The existing SELECT/WITH-only text guard in `postgres_query` stays unchanged (defense-in-depth alongside the DB-side `app_readonly` grants).

**Opt-in smoke test** — new `scripts/hosted-data-mcp-smoke.sh`: a JSON-RPC stdio handshake (same technique as `mcp_handshake` in `scripts/mcp-preflight.sh`) that starts the launcher, calls `data_status` + a trivial `postgres_query('SELECT 1')`, and tears down. **Not** part of the default `make mcp-preflight` / `make context-verify` loop, since it opens a real SSH session to production on every run — kept as an explicit, separate command.

```mermaid
flowchart LR
    Cursor["Cursor MCP client"] --> Launcher["hosted-data-mcp.sh (WSL)"]
    Launcher -->|"fail closed if ports busy or tunnel drops"| Tunnel["ssh -N -L 15432->5432 -L 16379->6379"]
    Tunnel -->|"SSH :22, existing key"| BearHost["BearHost VPS"]
    BearHost --> PG["Postgres loopback :5432 (app_readonly, statement_timeout=5s)"]
    BearHost --> Redis["Redis loopback :6379"]
    Launcher --> DataMcp["data_mcp.py --postgres-url 127.0.0.1:15432 --redis-url 127.0.0.1:16379"]
    DataMcp --> Tunnel
```

## 2. Hosted-aware health tool

Extend existing [`tools/stack/stack_mcp.py`](c:\Users\Aron\twitch-7tv-clone\tools\stack\stack_mcp.py) (which already has `http_request` and per-call `base_url` params) with a new tool:

```python
DEFAULT_HOSTED_BASE = "https://api.streampulse.stream"

@mcp.tool()
def hosted_health(mode: str = "auto") -> dict[str, Any]:
    """Check local (:8090) and/or hosted (api.streampulse.stream) health. mode: local|hosted|auto."""
```

`auto` checks both and labels which responded — matches the pattern already used in `scripts/context/backfill_status.sh` (`LOCAL_BASE`/`HOSTED_BASE`), now exposed as an MCP tool instead of only a shell snapshot. No new server needed; registers under the existing `streamclone-stack` entry.

## 3. Second codegraph instance for streamclone-pulse

`tools/codegraph/codegraph_mcp.py` already takes `--repo`/`--db` args and `ingest.py`'s `load_services()` returns `[]` gracefully if `tools/codegraph/subsystems.json` doesn't exist in the target repo — so a second instance is a new launcher, not a code change:

- New [`scripts/codegraph-pulse-mcp.sh`](c:\Users\Aron\twitch-7tv-clone\scripts\codegraph-pulse-mcp.sh): resolves `PULSE_REPO` (default `$ROOT/../streamclone-pulse`), fails clearly if that path doesn't exist, then runs the same venv against `--repo "$PULSE_REPO" --db "$PULSE_REPO/.codegraph/streamclone-pulse.kuzu"`. Registered as MCP server name **`streamclone-pulse-codegraph`** (distinct instance, not blended into the backend graph).
- `scripts/codegraph-pulse-mcp.ps1` mirroring `codegraph-mcp.ps1`.
- New Makefile targets `codegraph-pulse` (ingest) using `CODEGRAPH_PULSE_REPO ?= ../streamclone-pulse`. Skip generalizing `tools/codegraph/smoke.py` (it hardcodes backend-repo-specific thresholds/symbols) — validate the pulse instance via the standard MCP `tools/list` handshake in preflight instead of a full smoke assertion suite.
- Optional small seed file `streamclone-pulse/tools/codegraph/subsystems.json` (extension/portal keyword → path mappings, e.g. `extension` → `src/`, `portal`/`analytics-hub` → `streampulse-web/src/routes/analytics`) so `explain_subsystem` has something to seed on for this repo too.
- Update [`docs/agent-codegraph.md`](c:\Users\Aron\twitch-7tv-clone\docs\agent-codegraph.md) and [`streamclone-pulse/docs/CONTEXT.md`](c:\Users\Aron\streamclone-pulse\docs\CONTEXT.md) "Codegraph unavailable" section: replace "not indexed" with "indexed via a separate `streamclone-pulse-codegraph` MCP instance; Repomix/grep remain the fallback for anything the tree-sitter/heuristic pass misses."

## 4. Wire up `.cursor/mcp.json` in both repos

- Update the example files to add the two new servers:
  - [`.cursor/mcp.windows.json.example`](c:\Users\Aron\twitch-7tv-clone\.cursor\mcp.windows.json.example), `.cursor/mcp.recommended.json.example`, `.cursor/mcp.linux.json.example` — add `streamclone-hosted-data` (→ `scripts/hosted-data-mcp.sh`) and `streamclone-pulse-codegraph` (→ `scripts/codegraph-pulse-mcp.sh`). Leave `.cursor/mcp.minimal.json.example` untouched (it's intentionally minimal: codegraph + data only).
  - [`streamclone-pulse/.cursor/mcp.recommended.json.example`](c:\Users\Aron\streamclone-pulse\.cursor\mcp.recommended.json.example) — currently only lists `streamclone-codegraph` + `streamclone-data`; bring it in line with the backend's recommended set: add `streamclone-stack`, `playwright`, `streamclone-hosted-data`, `streamclone-pulse-codegraph` (all via the same `wsl.exe --cd ../twitch-7tv-clone bash scripts/...` pattern already used there).
- Actually copy the updated Windows example into a real, local `.cursor/mcp.json` in **both** repos (gitignored in both — confirmed no such file exists yet in either). This is the concrete "wire it up" step, not just documentation.

## 5. Preflight / discovery updates

[`scripts/mcp-preflight.sh`](c:\Users\Aron\twitch-7tv-clone\scripts\mcp-preflight.sh) and `scripts/mcp-list-tools.sh`:

- Add file-existence checks for the two new launcher scripts.
- Add a safe-by-default `tools/list` handshake for `streamclone-pulse-codegraph` (skips with a clear message if the sibling repo/DB isn't present yet).
- **Deliberately exclude** `streamclone-hosted-data` from the default handshake loop (it would open a real SSH session + prod DB connection on every `make context-verify`); document `MCP_PREFLIGHT_HOSTED=1 bash scripts/hosted-data-mcp-smoke.sh` as the explicit, separate way to verify it.

## 6. Docs

[`docs/MCP.md`](c:\Users\Aron\twitch-7tv-clone\docs\MCP.md):

- Fix drift: the "Codegraph tools" list currently shows 6 tools; the server exposes 12 (per `docs/agent-codegraph.md`'s table: `search_symbols`, `get_ast_chunk`, `get_call_chain`, `get_blast_radius`, `graph_status`, `rebuild_graph`, `find_callers`, `find_callees`, `find_routes`, `find_tests_for_symbol`, `impact_analysis`, `explain_subsystem`).
- Add `streamclone-hosted-data` and `streamclone-pulse-codegraph` rows to the "In-repo MCP servers" table and priority matrix.
- Add `hosted_health` to the "Stack tools" list.
- New short subsection "Hosted read-only data (BearHost)" covering: what it connects to, the `app_readonly` role + `statement_timeout`, the SSH-tunnel-only design (no public DB ports, no Tailscale DB exposure), where the password lives (`~/.streamclone/hosted-data-mcp.env`, never in `.cursor/mcp.json` or the repo), and that it's excluded from default preflight.

## Verification (Cursor sees the servers)

1. `bash scripts/mcp-preflight.sh` from `twitch-7tv-clone` — expect 4 automatic handshakes passing (codegraph, stack, data, pulse-codegraph) with non-zero tool counts; hosted-data reported as "skipped by default" not failed.
2. `MCP_PREFLIGHT_HOSTED=1 bash scripts/hosted-data-mcp-smoke.sh` — separate, explicit check that the tunnel + `app_readonly` role + `data_status`/`postgres_query('SELECT 1')` round-trip works end to end.
3. Reload Cursor; in Settings → MCP confirm 5 servers green with tool counts, in **both** the `twitch-7tv-clone` root and the `streamclone-pulse` root (or the multi-root `streamclone-pulse-extension.code-workspace`).
4. `git status` in both repos shows no new tracked files from the `.cursor/mcp.json` writes (still gitignored).

## Explicitly out of scope for this pass

- Redis ACL read-only user (Postgres gets a real DB-side role per your answer; Redis stays behind the same SSH tunnel with the existing default user — flagged as a possible follow-up, not required now).
- `.codex/config.toml.example` / `scripts/codex-mcp-launch.sh` updates for the two new servers (Cursor-only verification was requested; Codex parity can follow separately).
- Generalizing `tools/codegraph/smoke.py` to run its full repo-specific assertion suite against the pulse graph (kept to a basic handshake check instead).
