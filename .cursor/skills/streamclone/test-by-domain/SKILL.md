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
| Compose/env/deploy | `make compose-config-check`, `make security-scan` |
| Playback | `make test-video`, frontend build |
| Emotes | `make test-emote` |
| Metadata / directory | `make test-metadata` |
| Product boundary | `make product-boundary-strict` |
| Docs only | link check, `git diff --check -- '*.md'` |
| Stack up after edits | `make agent-smoke` |

Use `make check` before a broad PR when time allows.
