---
name: streamclone-test-by-domain
description: Pick narrow Streamclone tests from changed file paths. Use after any code change before claiming done, or when the user asks what tests to run.
---

# Streamclone test by domain

Read `.kiro/steering/tech.md` for defaults.

## Map changed paths → commands

| Paths touched | Run first |
|---------------|-----------|
| `internal/analytics/` | `go test ./internal/analytics/...` |
| `internal/chat/` | `go test ./internal/chat/...` |
| `internal/video/` | `go test ./internal/video/...` |
| `internal/metadata/` | `go test ./internal/metadata/...` |
| `internal/emote/` or emote worker | `go test ./internal/emote/...` (or matching package) |
| `cmd/*` | `go test ./cmd/...` then package tests for imported internals |
| `clipper/` | `make clipper-test` |
| `frontend/` | `cd frontend && npm run build` |
| `deploy/` | `docker compose --env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml config` |
| Cross-package / shared `internal/` | `go test ./...` after narrow passes |

## Proxy / auth / chat transport changes

Also verify:

- `http://localhost:8090/v1/auth/debug`
- WebSocket subscribe through `ws://localhost:8090/v1/ws`

## Playback / channel UI changes

Validate against **`http://localhost:8090`**, not standalone Vite `:5174`.

## Before finishing

1. Run the narrowest command that covers the diff
2. Escalate to full suite only when boundaries were crossed
3. Summarize pass/fail in prose — do not paste full test output
