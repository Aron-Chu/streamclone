# Storage migration tasks (Azure → R2)

Ledger for long-term artifact storage. **BearHost Postgres** remains source of truth for `archive_exports`, queues, saved moments, VOD Library rows, and hot indexes. **Production reads/writes still use Azure** until STOR-R2-003+ are shipped behind flags.

Plan: [azure-to-r2-migration.md](./azure-to-r2-migration.md) · Index: [README.md](./README.md)

---

## Current status (2026-06-25)

| Item | State |
|------|-------|
| Azure authoritative (production) | **Yes** — `AzureBlobStore` only in Go |
| R2 personal account | **Enabled** (`51dd8007…`) |
| Staging bucket | **`streampulse-artifacts-staging`** |
| Prod / backups buckets | **Not created** |
| One-object staging proof | **Done** — 131 B rollup; SHA-256 + gzip ok; Azure etag unchanged |
| Production read-path cutover | **No** |
| Bulk copy | **No** |

---

## Task ledger

| ID | Status | Summary | Guardrails |
|----|--------|---------|------------|
| **STOR-R2-001** | **Done** | Phase 0.6 inventory + Phase 2A: R2 enable, staging bucket, one-object mirror proof | Azure unchanged; no prod buckets; no read-path |
| **STOR-R2-002** | Pending | Mirror **10–50** confirmed rows from [`sample-manifest-phase2a.csv`](./sample-manifest-phase2a.csv) to staging | Tiny types only; no `postgres/nightly/`; no bulk `vod_chat/` |
| **STOR-R2-003** | Pending | Implement `R2BlobStore` + `ReadThroughStore` behind env flags (BearHost) | Default off; Azure fallback; no schema cutover in same PR without review |
| **STOR-R2-004** | Pending | R2 restore drill (read object from staging; compare to Azure; optional pg backup restore from R2 **after** backup class mirrored) | Block `postgres/nightly/` mirror until drill passes |
| **STOR-R2-005** | Pending | Batch migration by prefix after STOR-R2-002–004 pass | No Azure delete; no lifecycle change; 90d fallback window |

---

## Explicit deferrals

- **`postgres/nightly/`** → R2 backups bucket only after **STOR-R2-004** restore drill.
- **`vod_chat/` bulk** → after sample mirror + read-through proven on staging.
- **`streampulse-artifacts-prod` / `streampulse-backups-prod`** → operator approval only.
- **VOD Library rows, saved moments, progress, pins** → Postgres only, never R2.
- **Top 500 live tracking / queues** → Postgres + BearHost workers; R2 not on critical path (see streamclone-pulse `top-500-storage-infra.md`).
