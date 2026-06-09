# Streamclone stack MCP

Read-only MCP server for local stack diagnostics (`http://localhost:8090`).

## Setup

Uses the codegraph venv (includes `mcp`):

```sh
make codegraph-install
```

## Run

```sh
.codegraph/.venv/bin/python tools/stack/stack_mcp.py --repo .
```

Windows Cursor: `scripts/stack-mcp.ps1` (wraps WSL venv).

## Tools

- `stack_health` — auth debug, service health, container list
- `stack_ports` — port listeners (wslrelay detection)
- `playback_probe` — stream diagnostics + HLS through proxy
- `twitch_auth_status` — auth debug, `/v1/me`, clips probe
- `scraper_probe` — TwitchTracker scrape with meta#ecs check
- `compose_logs` — bounded docker compose logs
