# Phase C shadow deploy — operator checklist (streampulse-ops)

**Do not execute until Step 0 baseline + Step 1 preconditions pass.**

## 1. Release (streamclone repo)

```bash
# Local gate (already green in dev)
go test ./internal/analytics/ingestcore/... ./internal/analytics/...
go build ./cmd/analytics

# Tag + push per production-artifact-contract.md (operator)
# VERSION / IMAGE_TAG must match
```

Includes: ingest-core package, allowlist parse fix, shadow artifact rotation (128MiB), moments Cache-Control.

## 2. Deploy image (streampulse-ops)

- Pin new `IMAGE_TAG` in production manifest
- Run migrate if required
- Recreate analytics with **prior env first** if only shipping Cache-Control fix
- **Separate restart** for Phase C shadow env (do not merge with Docker limits change)

## 3. Phase C env overlay

Copy from [`deploy/env/profile-hosted-ingest-core-phase-c.env.example`](../../../deploy/env/profile-hosted-ingest-core-phase-c.env.example):

```env
INGEST_CORE_ENABLED=0
INGEST_CORE_DUAL_READ_MODE=1
INGEST_CORE_SHADOW_MODE=1
INGEST_TIERING_ENABLED=0
HUB_ROSTER_LIMIT=250
MAX_ACTIVE_IRC_CHANNELS=250
# INGEST_SHADOW_CHANNEL_ALLOWLIST=xqc,ludwig,tarik,kaicenat  # first hour only
```

## 4. Post-deploy verification

```bash
docker logs streamclone-analytics-1 2>&1 | grep 'ingest-core active'
docker exec streamclone-analytics-1 printenv INGEST_CORE_ENABLED INGEST_CORE_DUAL_READ_MODE INGEST_CORE_SHADOW_MODE
ls -la /opt/streamclone/app/runtime/ingest-shadow/
curl -s https://api.streampulse.stream/v1/public/hub?activityWindow=24h | jq .ingest
```

Expected:

- `core_enabled=false`, `dual_read=true`, `shadow=true` in startup log
- `ingest.coreEnabled=false` in hub JSON
- Shadow JSONL appears under `runtime/ingest-shadow/`
- Legacy PG writes unchanged (no ingest BatchFlusher production writes)

## 5. Allowlist expansion schedule

| Time | Action |
|------|--------|
| +1h | 5–10 channel allowlist |
| +3–6h | top 50 |
| +12h | remove allowlist (full 250) |

Restart analytics on each allowlist env change (same image).
