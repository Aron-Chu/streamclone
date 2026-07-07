---
name: api-contract-drift-check
description: Detect drift between extension/portal clients and streamclone BFF contracts. Use when changing extension_api.go, pulse-core types, portal routes, or shared message payloads.
---

# API contract drift check

## Surfaces that must agree

| Layer | Location |
|-------|----------|
| Go BFF | streamclone `internal/analytics/extension_api.go`, portal handlers |
| Shared types | streamclone `packages/pulse-core/` |
| Extension SW | `src/background/api.ts`, `src/shared/messages.ts` |
| Portal client | `streampulse-web/` apiClient (when present) |

## Workflow

1. Identify changed request/response fields in Go or TS.
2. Diff against `packages/pulse-core` adapters and extension message types.
3. Confirm portal uses `/v1/portal/analytics/*` sanitized paths — not raw extension payloads.
4. Run narrow tests:

```bash
# streamclone
go test ./internal/analytics/... -run Pulse
npm test --prefix packages/pulse-core

# streamclone-pulse
npm test
npm run typecheck
```

## Script

```bash
python .cursor/skills/api-contract-drift-check/scripts/contract-keys.py
```

Reports pulse-core export names vs common BFF JSON keys (heuristic, not exhaustive).

## Block merge if

- Extension computes scores or merges rollups client-side
- Portal reads unsanitized analytics fields
- Breaking field rename without pulse-core + both clients updated
