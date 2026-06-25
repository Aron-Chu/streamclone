# Storage migration tasks (Azure → R2)

Ledger for long-term artifact storage. **BearHost Postgres** remains source of truth for `archive_exports`, queues, saved moments, VOD Library rows, and hot indexes.

> **Production remains Azure-only.** BearHost production (`profile-bearhost-prod.env`) must keep `ARCHIVE_READ_THROUGH=false`, `ARCHIVE_DUAL_WRITE=false`, and `ARCHIVE_PRIMARY_PROVIDER=azure`. STOR-R2-004 passed locally; that does **not** authorize a production read-path flip without explicit operator approval.

Plan: [azure-to-r2-migration.md](./azure-to-r2-migration.md) · Index: [README.md](./README.md)

---

## Current status (2026-06-25)

| Item | State |
|------|-------|
| Azure authoritative (production) | **Yes** — default `NewBlobStore` → `AzureBlobStore` only |
| R2 Go support | **Implemented** — `R2BlobStore`, `ReadThroughStore`; **flags default off** |
| R2 restore drill (staging) | **Done** — [`r2-restore-drill-log.md`](./r2-restore-drill-log.md) |
| R2 personal account | **Enabled** (`51dd8007…`) |
| Staging bucket | **`streampulse-artifacts-staging`** |
| Prod / backups buckets | **Not created** |
| Staging sample mirror | **Done** — 31 objects; [`sample-mirror-phase2b.csv`](./sample-mirror-phase2b.csv) |
| Production read-path cutover | **No** — operator must enable read-through on BearHost separately |
| Prefix batch migration | **No** |

---

## Task ledger

| ID | Status | Summary | Guardrails |
|----|--------|---------|------------|
| **STOR-R2-001** | **Done** | Phase 0.6 inventory + Phase 2A: R2 enable, staging bucket, one-object mirror proof | Azure unchanged; no prod buckets; no read-path |
| **STOR-R2-002** | **Done** | Mirror **31** sample rows — [`sample-mirror-phase2b.csv`](./sample-mirror-phase2b.csv) | Tiny types only; no `postgres/nightly/`; no bulk `vod_chat/` |
| **STOR-R2-003** | **Done** | `R2BlobStore` + `ReadThroughStore` + `NewBlobStore` factory; env flags; unit tests | Defaults: Azure-only; no schema change; no prod flag flip |
| **STOR-R2-004** | **Done** | R2 restore drill — direct R2 read, read-through hit, Azure fallback, gzip; log [`r2-restore-drill-log.md`](./r2-restore-drill-log.md) | Local/staging only; no BearHost flag flip; no `postgres/nightly/` yet |
| **STOR-R2-005** | **Deferred** | Batch migration by prefix — not scheduled (see below) | No Azure delete; no lifecycle change; 90d fallback window |

---

## STOR-R2-005 — defer (2026-06-25)

**Recommendation: do not start batch migration now.**

| Factor | Current state |
|--------|---------------|
| Azure archive size | ~97 MiB / ~9.2k objects (Phase 0.6 inventory) |
| Azure cost | ~$0.13/mo at current scale — migration saves almost nothing |
| R2 mirror coverage | 31 sample objects (~5.2 KiB) — pipeline proven, not bulk copied |
| Production path | Azure-only; read-through drill passed **locally only** |
| Risk vs reward | Batch copy adds egress/ops work; no prod cutover benefit until scale or egress pain |

**Revisit STOR-R2-005 when:** Azure egress/cost spikes, prod read-through canary on BearHost is approved, or a feature explicitly needs R2 as primary for a prefix class.

---

## Explicit deferrals

- **`postgres/nightly/`** → R2 backups bucket only after **STOR-R2-004** restore drill.
- **`vod_chat/` bulk** → after sample mirror + read-through proven on staging.
- **`streampulse-artifacts-prod` / `streampulse-backups-prod`** → operator approval only.
- **VOD Library rows, saved moments, progress, pins** → Postgres only, never R2.
- **Top 500 live tracking / queues** → Postgres + BearHost workers; R2 not on critical path (see streamclone-pulse `top-500-storage-infra.md`).

## Env reference (STOR-R2-003)

See [azure-to-r2-migration.md § Phase 3](./azure-to-r2-migration.md) and `.env.example` (`ARCHIVE_PRIMARY_PROVIDER`, `ARCHIVE_READ_THROUGH`, `ARCHIVE_DUAL_WRITE`, `ARCHIVE_R2_*`).
