---
description: Diagnose local Streamclone stack health, ports, setup-control, optional services, and localhost routing.
---

# Stack Debug

Read `AGENTS.md`, `.kiro/steering/windows-dev.md`, and `.kiro/steering/tech.md`.

## Workflow

1. Probe `http://localhost:8090`.
2. Check containers, ports, and setup-control.
3. Avoid raw service ports unless intentionally bypassing Caddy.
4. Keep diagnostics redacted.

## Tools

- `stack_health`
- `stack_ports`
- `compose_logs`
- `twitch_auth_status`

## Checks

```sh
make ps
make compose-config-check
make security-scan
```
