# STOR-R2-004 restore drill log (2026-06-25)

Local/staging **read-only** drill. No `archive_exports` updates. No Azure or R2 object mutations.

## Env flags (local only)

| Variable | Value |
|----------|-------|
| `ARCHIVE_PRIMARY_PROVIDER` | `azure` |
| `ARCHIVE_READ_THROUGH` | `true` |
| `ARCHIVE_DUAL_WRITE` | `false` |
| `ARCHIVE_R2_BUCKET` | `streampulse-artifacts-staging` |
| `ARCHIVE_R2_PREFIX` | `archive` |
| `ARCHIVE_R2_LIVE_TEST` | `1` |
| `ARCHIVE_AZURE_CONNECTION_STRING_FILE` | `~/.streamclone/azure-archive-connection-string` |
| R2 credentials | `~/.streamclone/r2-staging-s3.env` → ephemeral key files (not committed) |

Production BearHost: **unchanged** — all R2 flags remain default off.

> **Do not enable read-through on BearHost production** without explicit operator approval. This drill validates the Go path locally; production remains Azure-only.

## Sample objects exercised

From [`sample-mirror-phase2b.csv`](./sample-mirror-phase2b.csv):

| artifact_type | blob key (relative) | sha256 (compressed) | Tests |
|---------------|---------------------|---------------------|-------|
| `analytics_rollups` | `rollups/stream_id=316787476195/part-000.jsonl.gz` | `8c0fd0d6…519056` | direct R2, read-through R2 hit, gzip |
| `analytics_stream` | `streams/stream_id=316070541810/session.json.gz` | `17933819…a677b` | direct R2, read-through R2 hit, JSON session shape |
| `bronze_vod_catalog` | `channels/vod_index/gorizontradio.jsonl.gz` | `30e6fa98…6c7f` | direct R2, read-through R2 hit, gzip (stub `[]` payload) |

## Azure fallback

| Key | R2 | Azure | ReadThroughStore |
|-----|----|-------|------------------|
| `rollups/stream_id=317014684259/part-000.jsonl.gz` | **miss** (not in Phase 2B mirror) | **hit** | returns Azure bytes |

Auto-probed by `scripts/storage/r2-restore-drill.sh` when `ARCHIVE_DRILL_AZURE_FALLBACK_KEY` is unset.

## Command

```bash
bash scripts/storage/r2-restore-drill.sh
# equivalent:
ARCHIVE_R2_LIVE_TEST=1 go test ./internal/archive/... -run TestR2RestoreDrillLive -count=1 -v
```

## Result (2026-06-25)

```
--- PASS: TestR2RestoreDrillLive (1.77s)
    --- PASS: direct_r2_analytics_rollups
    --- PASS: read_through_r2_hit_analytics_rollups
    --- PASS: direct_r2_analytics_stream
    --- PASS: read_through_r2_hit_analytics_stream
    --- PASS: direct_r2_bronze_vod_catalog
    --- PASS: read_through_r2_hit_bronze_vod_catalog
    --- PASS: azure_fallback
r2-restore-drill: pass
```

Unit tests without live flag: `go test ./internal/archive/...` — **PASS**.

## Not in scope (deferred)

- `postgres/nightly/` restore from R2 (no backup class mirrored yet)
- BearHost production read-through enablement
- Batch prefix migration (**STOR-R2-005**)

## Mutations confirmation

**No Azure blob writes/deletes. No R2 uploads/deletes. No Postgres schema or `archive_exports` changes.**
