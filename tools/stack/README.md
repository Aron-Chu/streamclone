# Stack MCP

Read-only diagnostics for the local stack at `http://localhost:8090`.

Setup:

```sh
make codegraph-install
```

Run:

```sh
.codegraph/.venv/bin/python tools/stack/stack_mcp.py --repo .
```

Tools:

- `stack_health`
- `stack_ports`
- `playback_probe`
- `twitch_auth_status`
- `scraper_probe`
- `compose_logs`
