# Windows Dev Steering

Canonical local URL: `http://localhost:8090`.

## Common Problems

| Symptom | Check |
|---------|-------|
| `localhost:8090` fails but containers run | WSL/Windows port relay |
| Caddy exits on mount | `deploy/Caddyfile.local-tunnel` became a directory |
| Release install lacks source fix | Docker image tag/version not updated |
| Optional service buttons fail | setup-control token or daemon |

## Commands

```powershell
powershell -File scripts\setup.ps1
powershell -File scripts\start-streamclone.ps1
powershell -File scripts\check-streamclone.ps1
powershell -File scripts\validate-env.ps1
```

WSL/dev:

```sh
make up
make down
make compose-config-check
make security-scan
```

## Rules

- Release install lives at `%USERPROFILE%\streamclone`.
- Source checkout changes do not update built Docker images.
- Start/repair paths should fix localhost relays and Caddyfile mount state.
- Keep `.env.local` for local overrides and secrets.

## Before Finishing

- Install/script changes: test Start, Stop, Check, and Update paths where possible.
- Compose/env changes: run `make compose-config-check`.
- Security-sensitive changes: run `make security-scan`.
