---
description: Choose focused Streamclone checks for a change by touched domain.
---

# Test By Domain

Read `AGENTS.md` and `.kiro/steering/tech.md`.

## Map

| Change | Checks |
|--------|--------|
| Go shared/backend | `make test`, `make vet` |
| Frontend | `cd frontend && npm run build && npm test` |
| Clipper | `make clipper-test` |
| Compose/env/deploy | `make compose-config-check`, `make security-scan` |
| Playback | `go test ./internal/video/...`, frontend build |
| Analytics | `go test ./internal/analytics/...`, frontend build |
| Docs only | link check, `git diff --check -- '*.md'` |

Use `make check` before a broad PR when time allows.
