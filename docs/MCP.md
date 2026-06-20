# MCP guide for agents



Streamclone ships **read-only, repo-local** MCP servers for code navigation, stack diagnostics, and database inspection. Enable them in Cursor **Settings → MCP**; never commit your local `.cursor/mcp.json`.



**Philosophy:** read-heavy by default; write only through normal git edits; DB read-only; Docker access is logs/health/probes first.



---



## Priority matrix



| Priority | Tool | Why |

|----------|------|-----|

| **Essential** | `streamclone-codegraph` | Navigate Go/React symbols, blast radius, call chains |

| **Essential** | `streamclone-stack` | Docker health, ports, playback probes, bounded logs |

| **Essential** | `streamclone-data` | Read-only Postgres/Redis inspection |

| **Essential** | Playwright MCP | Verify UI, diagnostics, playback states |

| **Essential** | GitHub (Cursor plugin or [GitHub MCP](https://github.com/github/github-mcp-server)) | PRs, issues, CI, diffs — prefer read-only tool scope |

| **Nice** | Grafana MCP | Pulse dashboards when `pulse` profile + `:3000` active |

| **Nice** | Official docs lookup | Streamlink, MediaMTX, hls.js, ffmpeg, Twitch API |

| **Later** | Kubernetes MCP | Only if deploying on k8s regularly |

| **Later** | InfluxDB MCP | Only if Influx becomes core workflow |



### Avoid



- Full **write-access Docker** MCP — use `compose_logs` / `stack_health` instead

- Broad **filesystem** MCP when codegraph covers navigation

- Browser MCP **logged into personal accounts** by default

- MCP with **secret write** access unless heavily scoped

- Cloudflare **“bypass”** tooling — keep scraper debugging legitimate and observable



---



## In-repo MCP servers



| Server | Script | Purpose |

|--------|--------|---------|

| **streamclone-codegraph** | `scripts/codegraph-mcp.sh` | AST/symbol graph over Go + TS |

| **streamclone-stack** | `scripts/stack-mcp.sh` | Compose health, ports, playback, auth, scraper, **logs** |

| **streamclone-data** | `scripts/data-mcp.sh` | Read-only Postgres/Redis, emote jobs |



Implementation: `tools/codegraph/`, `tools/stack/`, `tools/data/`.



**Read-only logs:** stack MCP `compose_logs(service, tail?)` is the logs MCP — allowlisted services only, bounded tail (no arbitrary `docker exec`).



### Codegraph tools



- `get_ast_chunk(symbol)` — source slice for a function/type

- `get_blast_radius(symbol)` — callers and affected files

- `get_call_chain(symbol, depth)` — call tree

- `search_symbols(query, kind?, limit?)` — fuzzy search

- `graph_status()` — DB stats

- `rebuild_graph()` — triggers `make codegraph` (expensive; ask before running in shared env)



Requires: `make mcp-setup` or `make codegraph-install && make codegraph` → `.codegraph/streamclone.kuzu`.



### Stack tools



- `stack_health(base_url?)` — auth debug, service `/healthz`, container list

- `stack_ports()` — listeners on 8090, 8081–8086, DB ports; detects `wslrelay`

- `playback_probe(channel)` — stream diagnostics + HLS manifest via proxy

- `twitch_auth_status()` — `/v1/auth/debug`, `/v1/me`, clips probe

- `scraper_probe()` — TwitchTracker scrape direct vs proxy

- `compose_logs(service, tail?)` — bounded docker logs (metadata, video, chat, analytics, emote, storygraph, scraper, local-proxy, mediamtx, influxdb, …)



Default base URL: `http://localhost:8090`.



### Data tools



- `data_status()` — Postgres/Redis reachability

- `postgres_query(query, limit?)` — **SELECT / WITH only**, row cap 200

- `emote_jobs(limit?)` — processing job queue

- `redis_get(key)` — string/hash preview

- `redis_channel_emotes(login)` — channel emote dictionary hash



---



## Enable safely



1. **One command setup**



   ```sh

   make mcp-setup

   ```



2. **Copy recommended config — do not commit your local file**



   - All platforms: [`.cursor/mcp.recommended.json.example`](../.cursor/mcp.recommended.json.example) (Essential tier: codegraph + stack + data + playwright)

   - Linux / WSL: [`.cursor/mcp.linux.json.example`](../.cursor/mcp.linux.json.example)

   - Windows → WSL: [`.cursor/mcp.windows.json.example`](../.cursor/mcp.windows.json.example)

   - Merge into `.cursor/mcp.json` (gitignored) or Cursor UI.



3. **Preflight**



   ```sh

   bash scripts/mcp-preflight.sh

   ```



   Windows: `.\scripts\mcp-preflight.ps1`



4. **Reload Cursor** — confirm servers show green with tools listed.



### Optional: GitHub MCP



Prefer **Cursor GitHub integration** for PR/issue workflows. If using [GitHub MCP server](https://github.com/github/github-mcp-server), scope tokens read-only where possible; never commit PATs.



### Optional: Grafana MCP



Enable only when Pulse profile is running (`make pulse-on`) and you need dashboard queries. See [`deploy/grafana/`](../deploy/grafana/).



### Optional: Playwright (Essential for UI verification)



Already in `mcp.recommended.json.example`. Also use:



```sh

make smoke-ui

make agent-smoke    # stack up

```



Playwright tests: `frontend/tests/playwright/`.



---



## CI codegraph artifact



When `internal/`, `frontend/src/`, or `cmd/` change, CI rebuilds the graph and uploads `.codegraph/streamclone.kuzu` (7-day retention). Download from the GitHub Actions run to debug symbol drift; **local `make codegraph` is still preferred** for day-to-day work.



---



## Windows vs WSL



- Wrapper scripts (`scripts/*-mcp.ps1`) launch **WSL bash**.

- Port **8090** owned by `wslrelay` → `.kiro/steering/windows-dev.md`; `stack_ports` warns.

- Do **not** commit absolute Windows paths — use `${workspaceFolder}`.



---



## Warnings



- **Do not commit** `.cursor/mcp.json`, `.env`, tokens, or API keys in MCP env blocks.

- `postgres_query` is read-only by design.

- `rebuild_graph` and scraper benchmarks can be CPU-heavy.



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



Figma bridge (design only): `.cursor/mcp.figma-bridge.json.example`.
