---
name: api-contract-drift-check
description: Detect drift between extension/portal clients and streamclone BFF contracts. Use when changing extension_api.go, pulse-core types, portal routes, or shared message payloads.
---

# API contract drift check

## Surfaces that must agree

| Layer | Location |
|-------|----------|
| Go BFF | `internal/analytics/extension_api.go`, portal handlers |
| Shared types | `packages/pulse-core/` |
| Extension SW | sibling `streamclone-pulse/src/background/api.ts`, `src/shared/messages.ts` |
| Portal client | sibling `streamclone-pulse/streampulse-web/` (when present) |

## Workflow

1. Identify changed request/response fields in Go or TS.
2. Diff against `packages/pulse-core` adapters and extension message types.
3. Confirm portal uses `/v1/portal/analytics/*` sanitized paths — not raw extension payloads.
4. Run narrow tests:

```bash
go test ./internal/analytics/... -run Pulse
npm test --prefix packages/pulse-core
# streamclone-pulse checkout:
npm test && npm run typecheck
```

## Script

From streamclone repo root (or set `STREAMCLONE_ROOT`):

```bash
python .cursor/skills/pulse/api-contract-drift-check/scripts/contract-keys.py
# Windows Store python alias fallback:
wsl.exe --cd /mnt/c/Users/Aron/twitch-7tv-clone bash -lc 'python3 .cursor/skills/pulse/api-contract-drift-check/scripts/contract-keys.py'
```

Strict contract tests (fail on drift):

```bash
go test ./internal/analytics/... -run Contract
cd ../streamclone-pulse && npm test -- coverageContract
cd streampulse-web && npm test -- publicHub.contract
```

## Block merge if

- Extension computes scores or merges rollups client-side
- Portal reads unsanitized analytics fields
- Breaking field rename without pulse-core + both clients updated
