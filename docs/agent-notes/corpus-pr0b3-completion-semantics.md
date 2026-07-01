# PR 0B-3 — Gold completion semantics (design note, read-only)

**Status:** Design only — **no runtime code in this note.**
**Depends on:** PR 0B-2 (`GOLD_VOD_SEGMENTS_ENABLED` wiring) deployed with canary soak.

---

## 1. Current completion path (today)

```text
BackfillWorker.runOnce (gold tiers)
  -> SyncHistoricalStream
    -> fetchVODCommentsParallel
      -> in-memory segment queue + checkpoints
      -> (0B-2) gold_vod_segments claim/complete/fail when flag on
  -> resolveGoldBackfillOutcome
    -> syncErr == nil  =>  backfill_jobs.status = done
```

`top500_vod_inventory.gold_status` is updated elsewhere (inventory/enqueuer paths) and is **not** yet derived from the durable segment ledger.

**Gap:** A job can reach `done` when `SyncHistoricalStream` returns nil even if durable segments remain `failed`, stale `running`, or `queued` (especially after worker crash or hot-split partial windows).

---

## 2. Where 0B-2 writes segment rows

| Event | Store call | Row effect |
|-------|------------|------------|
| Parallel fetch start | `UpsertGoldVODSegmentPlans` | `queued` rows per window |
| Worker picks segment | `ClaimGoldVODSegmentByKey` | `running` + lease |
| Segment success | `CompleteGoldVODSegment` | `done` |
| Segment error | `FailGoldVODSegment` | `failed` or `dead_letter` |
| Hot split | `FailGoldVODSegment` (old key) + upsert child windows | Parent rescheduled; tail `queued` |

---

## 3. Statuses that should block job `done`

| Segment status | Block `done`? | Notes |
|----------------|---------------|-------|
| `failed` | **Yes** | Retriable until `next_run_at`; still unresolved |
| `dead_letter` | **Yes** | Operator requeue required (PR 2) |
| `queued` | **Yes** | Expected windows not yet fetched |
| `running` + valid lease | **Yes** | Work in flight |
| `running` + expired lease | **Yes** | Treat as reclaimable incomplete (same as queued/failed for completion) |

**Grace period (proposed):** `queued`/`failed` with `next_run_at > now()` may still block `done` if the job's sync returned success prematurely — 0B-3 should not mark `done` until **all expected segment keys** for the VOD are terminal.

---

## 4. Statuses that allow job `done`

| Segment status | Allow `done`? | Notes |
|----------------|---------------|-------|
| `done` | **Yes** | Terminal success |
| `skipped` | **Yes** | Only with explicit sanitized reason (e.g. operator skip, VOD deleted) |
| Known-empty window | **Yes** | `done` + `comments_fetched = 0` + empty error + successful fetch evidence (requirements FR-2) |

Absence of rollup rows alone is **never** sufficient for known-empty.

---

## 5. `partial` status

**Recommendation:** Do **not** add a new `partial` enum value in 0B-3. Use:

- `failed` + retriable error for in-progress retry windows.
- `done` only when window fetch completed (including empty).
- Hot-split parent uses `hot_split rescheduled` fail + new keys for children.

If product later needs `partial` on `top500_vod_inventory`, model via `gold_status` + segment aggregate counts, not a new segment status.

---

## 6. Race avoidance

**Problem:** Worker A completes last in-memory segment and returns `nil` from `SyncHistoricalStream` while worker B still holds a `running` lease, or while `queued` tail rows exist after hot-split.

**Proposed hook (0B-3):**

1. Before `resolveGoldBackfillOutcome` marks `done`, when `GOLD_VOD_SEGMENTS_ENABLED` and Gold tier:
   - Query `gold_vod_segments` for `vod_id` (from job/stream) where status ∉ (`done`, `skipped`) OR (`running` AND lease valid).
2. If any blocking rows: return retriable error from sync **or** override outcome to `queued`/`failed` with `errMsg` referencing segment summary.

**Alternative:** Check inside `fetchVODCommentsParallel` return — if incomplete durable segments, return error even when in-memory queue is empty (aligns with existing `incompleteSegmentCount` error).

Prefer **single choke point** in `resolveGoldBackfillOutcome` or `BackfillWorker` finish path so Silver paths are untouched.

---

## 7. Transactions

**Recommendation:**

- Segment row updates: already per-statement atomic.
- Job finish + inventory update: **optional** single transaction in 0B-3 only if `top500_vod_inventory` update moves into same hook; otherwise keep job update separate with idempotent status checks.

Do not hold locks across GQL fetch. Completion check is a read before write on `backfill_jobs`.

---

## 8. Tests required (0B-3)

| Test | Type |
|------|------|
| Job blocked when segment `failed` | Integration |
| Job blocked when segment `dead_letter` | Integration |
| Job blocked when `running` lease not expired | Integration |
| Job allowed when all segments `done` | Integration |
| Job allowed known-empty (`done`, comments_fetched=0) | Unit + integration |
| Hot-split tail `queued` blocks job `done` | Integration |
| Flag off: existing `syncErr == nil` behavior unchanged | Unit |
| `top500_vod_inventory.gold_status` not set `done` when segments block | Integration (if hook added) |

---

## 9. Out of scope for 0B-3

- PR 2 gap list / requeue API
- PR 1 worker identity
- Admin UI
- Enabling flag globally without soak checklist (`corpus-0b2-hosted-verify.md`)
