# Corpus scaling + observability — requirements & forward plan

**Status:** Active planning source of truth (approved with wording fixes). **Do not implement corpus production changes until PR 0 (docs) and Gate 0 are complete.**

**Supersedes for agents:** ad-hoc corpus advice, stale “corpus deleted” claims, and rebuild-from-scratch segment-queue proposals that ignore existing code.

**Related docs:**

| Doc | Role |
|-----|------|
| [`docs/live-first-pivot.md`](../live-first-pivot.md) | Product direction (live-first public graph; manual VOD import; corpus as enrichment plane) — **must stay consistent with this file** |
| [`docs/streampulse-vps.md`](../streampulse-vps.md) | Where Silver/Gold workers run today |
| [`docs/MCP.md`](../MCP.md) | Hosted read-only inspection (`streamclone-hosted-data`) |
| [`docs/agent-notes/ivr-gold-prod-status.md`](../agent-notes/ivr-gold-prod-status.md) | Runtime snapshot (hub queue counts, IVR off in prod) |
| [`docs/storage/README.md`](../storage/README.md) | Postgres vs blob storage split |
| `.cursor/plans/hosted_mcp_+_gap_fixes_813bb858.plan.md` | MCP implementation checklist (Cursor plan) |

---

## North star

### Product

> StreamPulse is **live-first** in public surfaces. Gold/Silver are a **controlled historical enrichment plane**. Every corpus-derived number must be **source-labeled**, **coverage-backed**, and **never silently mixed** into live-only views.

### Engineering

> Make Gold/Silver **durable and observable** before scaling workers or buying proxies. Use **read-only hosted MCP** to inspect production truth before changing corpus behavior.

---

## Three planes (do not conflate)

| Plane | What it is | Default host | User trigger | Public exposure |
|-------|------------|--------------|--------------|-----------------|
| **Live Pulse** | Helix viewer sampler + IRC collector → minute rollups (`chat_source=live`) | BearHost API + streampulse-vps collector | Automatic for tracked roster | Global/channel activity graphs (live-only filter) |
| **Manual VOD import** | Extension/portal `PulseBackfillManager` → GQL chat for one VOD | BearHost `analytics` | User clicks “load missed moments” / backfill API | Per-session imported view; labeled `chat_source=gql` |
| **Autonomous corpus (Silver/Gold)** | Bronze index → Silver TT charts → Gold GQL chat backfill at scale | **streampulse-vps** (`scraper` + `analytics-workers`) | Operator config (`CORPUS_WORKERS_ENABLED=1`) | Hub **aggregate-only** tier counts (`corpusPipeline.silver\|gold`); no per-stream errors on public API |

**Corpus re-enable (2026-06-30):** Autonomous Silver/Gold workers run on streampulse-vps only. BearHost API keeps `CORPUS_WORKERS_ENABLED=false`. Manual VOD import stays on BearHost. See [`docs/live-first-pivot.md`](../live-first-pivot.md) § “Corpus re-enable”.

**Doc drift warning:** Older drafts of `live-first-pivot.md` claimed the corpus Go pipeline was deleted. **That is false in the current tree.** Agents must trust **this file + filesystem + hosted MCP queries**, not stale purge notes, before deleting or rebuilding corpus code.

---

## Public surface contract

| Surface | Allowed data | Forbidden |
|---------|--------------|-----------|
| **Live activity graph** (public hub) | Live IRC/Helix rollups only (`chat_source` live/mixed) | Autonomous Silver/Gold rows, manual-import GQL rows, IVR rows — unless the UI is an **explicitly labeled** historical/corpus/imported view |
| **Corpus pipeline block** (`corpusPipeline.silver\|gold`) | Aggregate-only Silver/Gold queue/readiness **counts** (pipeline health) | Per-stream gaps, logins, stream IDs, job errors, worker IDs, segment rows |
| **Historical / archive / imported pages** | Gold/Silver/GQL rows with **source**, **confidence**, and **coverage** labels | Presenting corpus/import data as live network activity |
| **Internal corpus APIs** (auth-gated) | Stream/VOD IDs, login, offsets, segment status, lease owner, attempts, sanitized errors | Raw chat text, chatter usernames, Twitch user IDs, viewer identities, viewer-level rankings |

### Public surface rule

`corpusPipeline.silver|gold` counts may appear on the public hub as **aggregate pipeline health**.

Those counts do **not** mean corpus rows may feed the live activity graph.

The live activity graph must remain **live-only** unless the UI creates an explicitly labeled historical/corpus/imported view.

---

## Tier definitions (project vocabulary)

| Tier | Source | Answers | Final storage (canonical) |
|------|--------|---------|---------------------------|
| **Bronze** | Helix VOD index + TwitchTracker stream rows | Stream/VOD identity, session metadata | Postgres stream/session tables + optional Azure bronze blobs |
| **Silver** | TwitchTracker viewer charts | Viewer curve, avg/peak, session shape | `analytics_minute_rollups` viewer fields + `backfill_jobs` tier=silver |
| **Gold** | Twitch GQL VOD comments (+ emote tokenization) | Chat velocity, emote bursts, Pulse Moments, replay heatmap | `analytics_minute_rollups` chat/emote fields (`chat_source=gql` when stamped) |
| **Live IRC** | In-process / pulse-collector | Real-time chat for tracked channels | Same rollup table, `chat_source=live` |

Gold also has an **optional IVR accelerator** (`gold_ivr.go`, `logs.ivr.fi`) — off in prod today; shadow/canary only until explicitly deployed.

---

## Current state (audit summary, 2026-07-01)

Evidence from repo + `origin/master` archaeology. Local checkouts may be behind upstream — **sync `master` with `origin/master` before any migration work.**

### What already works (do not rebuild)

| Capability | Location | Notes |
|------------|----------|-------|
| Whole-job multi-host claim | `backfill_worker.go` `claimNext()` | Transaction + SKIP LOCKED-style claim on `backfill_jobs` |
| Gold in-process segment fetch | `sync_gql_parallel.go` | Adaptive 300s/120s segments, hot-split, priority heap, parallel + serial retry |
| Same-process crash resume | `analytics_sync_checkpoints.segments_json` | Not multi-worker-safe |
| GQL comment dedupe | `gqlCommentDeduper` + tests | Cross-page and segment-boundary |
| Silver/Gold enqueue | `silver_enqueuer.go`, `gold_enqueuer.go`, `top500_gold_vod_inventory.go` | Top-500 can enqueue Gold without Silver prerequisite |
| Corpus readiness API | `GET /v1/corpus/readiness`, `/v1/internal/corpus/readiness` | Tier + `goldSegments` aggregate counts |
| Public hub corpus block | `hub_overview.go` `HubCorpusPipeline` | Aggregate-only; test-guarded |
| Top-500 VOD inventory | `top500_vod_inventory` + migration `000047` | Per-VOD `gold_status`, not segment-level |
| TT backoff (two-tier) | `tt_scrape_backoff.go` | Global only for 4 shared-resource reasons; else per-stream |

### Half-finished (resurrect, do not replace)

| Item | State | Action |
|------|-------|--------|
| **`gold_vod_segments`** | Table + store (`gold_vod_segment_store.go`) designed; migration **`000049`** on `origin/master`; **zero production callers** | Wire `Claim`/`Complete`/`Fail` into Gold path (PR 0B) |
| **`gold_vod_rate_limits`** | Referenced in `CorpusGoldSegmentSummary` SELECT only; **no CREATE TABLE anywhere** | Design in PR 0A or remove dead query (PR 0B) |

### Gaps (real risks)

| Gap | Impact |
|-----|--------|
| Gold job `done` = `SyncHistoricalStream` returned nil | Whole-job status; in-process fetch already errors on incomplete segments, but **process death** loses in-memory state |
| No durable segment ledger in production | Cannot prove per-window coverage; cannot safely split one VOD across hosts |
| No worker identity (`SCRAPER_ID` / `GQL_WORKER_ID`) | Premature until second identity exists |
| `gold_vod_segments` tests gated behind `INTEGRATION=1` | CI does not catch migration/store drift |
| `docs/live-first-pivot.md` purge section vs filesystem | Agent may delete live production code |

---

## Non-negotiable rules

### Product & API

- **Public APIs** must not expose stream IDs, logins, job errors, segment detail, worker IDs, or per-stream corpus gaps.
- **Internal authenticated APIs** may expose stream IDs, VOD IDs, logins, offsets, lease owners, status, attempts, and sanitized errors for operator debugging.
- **No API, public or internal,** may expose raw chat text, chatter usernames, Twitch user IDs, viewer identities, or viewer-level rankings.
- Corpus-derived rollups must remain **source-labeled** (`chat_source`, `source_confidence` per migration `000050`).
- The **live activity graph** must not silently include autonomous Gold/Silver or manual-import rows (see `AggregateActivityBucketsSince` live filter in [`docs/live-first-pivot.md`](../live-first-pivot.md)). `corpusPipeline` counts are pipeline health, not live activity data.

### Engineering

- **Read-only planning first** for each PR; use hosted MCP SELECTs to validate assumptions on prod.
- **Do not delete corpus code** based on stale docs.
- **Do not rebuild** segment sizing, splitting, retries, checkpoints, dedupe, whole-job claiming, or Top-500 inventory.
- **Do not invent** a new Gold segment table if `gold_vod_segments` matches the design (it does).
- **Sync `master` with `origin/master`** before authoring migrations (avoid `000049` collision).
- **No write-capable production DB MCP** — `streamclone-hosted-data` is inspect-only (`app_readonly`, SELECT/WITH guard).
- **No laptop scraper/corpus** unless an explicit ADR reverses `.kiro/steering/laptopworker-hosting.md`.
- **No second worker identity** until PR 0B (durable segments) + PR 2 (gap detection) land.
- **No mobile proxies** unless hosted metrics show Cloudflare/403/429 dominates **after** identity-scoped backoff is measured.

---

## MCP track (prerequisite for safe corpus work)

Goal: agents inspect **hosted truth** before changing corpus production behavior.

| Server | Purpose | Mutation |
|--------|---------|----------|
| `streamclone-hosted-data` | BearHost Postgres/Redis via SSH tunnel + `app_readonly` | **Never** — SELECT/WITH only |
| `streamclone-stack` `hosted_health` | Compare local `:8090` vs `api.streampulse.stream` | Read-only |
| `streamclone-pulse-codegraph` | Index `streamclone-pulse` (portal/extension) | Read-only |
| `streamclone-codegraph` | Backend Go + `packages/*` | Read-only |

**Operator setup:** [`docs/MCP.md`](../MCP.md) § Hosted read-only data.

**Corpus inspection queries (examples — run via hosted MCP after PR 0B):**

```sql
-- Backfill queue health
SELECT tier, status, count(*) FROM backfill_jobs GROUP BY 1, 2;

-- Gold segment ledger (empty until PR 0B wired)
SELECT status, count(*) FROM gold_vod_segments GROUP BY 1;

-- Top-500 VOD gold status
SELECT gold_status, count(*) FROM top500_vod_inventory GROUP BY 1;
```

**Explicitly out of scope for MCP:** requeue, segment claim, backoff clear, or any production write.

Implementation status: see `.cursor/plans/hosted_mcp_+_gap_fixes_813bb858.plan.md` (most items completed; verify step may still be in progress).

---

## Functional requirements

### FR-1 Durable Gold segment progress

- Segment state must survive **worker process death** and be claimable by **another process** on the same DB.
- Reuse `gold_vod_segments` + existing store methods; preserve in-memory `sync_gql_parallel` adaptive logic.
- Hot-split segments must **upsert** new rows (new `segment_key`) without corrupting completed windows.

### FR-2 Honest completion

- A Gold `backfill_jobs` row must not reach `status=done` while expected `gold_vod_segments` for that VOD remain in `failed`, `dead_letter`, or stale `queued`/`running` (expired lease).
- Distinguish **known-empty** from **missing** in coverage reporting (PR 2).

**Known-empty** requires durable fetch evidence:

- Segment reached terminal success state (`done` or equivalent).
- `comments_fetched = 0` or equivalent successful empty result.
- No terminal fetch error on the segment row.
- Segment window exists in `gold_vod_segments`.

Absence of rollup rows alone is **never** enough to mark known-empty. A quiet minute and missing data can look identical in final rollups.

### FR-3 Silver identity backoff (deferred)

- Add identity dimension only to the **four** global TT shared-resource backoff keys once a second scraper host exists.
- Per-stream backoff remains unchanged.

### FR-4 Coverage observability

- Internal operators can see per-stream Silver/Gold coverage bars and a gap list.
- Public hub continues to show **tier aggregates only**.

### FR-5 Requeue

- Segment-level requeue for failed/missing Gold windows (internal API only).
- Whole-job requeue remains via existing `ReclaimStaleRunningJobs` / backfill retry paths.

### FR-6 Public/live graph isolation

- Public live graph queries must exclude autonomous Gold/Silver and manual-import rows unless the view is explicitly labeled historical/corpus/imported.
- `corpusPipeline` counts are pipeline health, not live activity data.
- Tests must guard that the public hub does not leak internal corpus detail (existing `TestPublicHubResponseOmitsSensitiveKeys` pattern).

---

## Non-functional requirements

| ID | Requirement |
|----|-------------|
| NFR-1 | Idempotent rollup writes (existing dedupe preserved) |
| NFR-2 | Safe multi-host at job level today; segment level after PR 0B |
| NFR-3 | Low ops overhead — extend `/v1/internal/corpus/readiness`, do not fork parallel systems |
| NFR-4 | `INTEGRATION=1` Gold segment tests run in CI or documented nightly gate |
| NFR-5 | Metrics: segment status counts, failures by reason, oldest queued segment age |

---

## Data model

### Reuse (canonical analytics — do not change semantics)

- `analytics_minute_rollups` — final chat/viewer/emote aggregates
- `backfill_jobs` — whole-job queue (silver/gold/gold_lite)
- `top500_vod_inventory` — per-VOD gold_status for Top-500 path
- `analytics_sync_checkpoints` — single-worker GQL resume (keep alongside segment ledger)

### Resurrect (PR 0B)

**`gold_vod_segments`** — schema on `origin/master` migration `000049_gold_vod_segments.up.sql`:

- Status enum: `queued`, `running`, `done`, `failed`, `dead_letter`, `skipped`
- Lease: `lease_owner`, `lease_expires_at`, `heartbeat_at`
- Progress: `cursor`, `comments_fetched`, `attempt`, `max_attempts`, `next_run_at`, `error`
- FK: `backfill_job_id` → `backfill_jobs(id)`
- Unique: `segment_key`; window unique on `(vod_id, start_offset_seconds, end_offset_seconds, strategy_version)`

`lease_owner` accepts worker identity strings (e.g. `streampulse-vps-0`) without schema change.

`gold_vod_segments.login` is **internal/operator metadata only**. It must never be exposed through `/v1/public/*`.

### TBD (PR 0A decision)

**`gold_vod_rate_limits`** — either add migration with explicit columns matching a designed rate-limit bucket model, **or** remove the dead SELECT from `CorpusGoldSegmentSummary` and rely on Prometheus + in-process `gqlRateCoordinator`.

---

## API requirements

### Public (unchanged sensitivity)

- `/v1/public/hub` — aggregate pipeline counts and live activity graph only; no segment detail.
- Existing hub tests remain the guardrail.

### Internal (extend)

Extend existing routes where possible:

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/v1/internal/corpus/readiness` | **Exists** — ensure `goldSegments` reflects wired ledger |
| GET | `/v1/internal/corpus/streams/{stream_id}/coverage` | Per-stream segment grid + coverage % |
| GET | `/v1/internal/corpus/gaps` | Cross-stream failed/missing segments |
| POST | `/v1/internal/corpus/gaps/requeue` | Reset selected segment rows to `queued` (auth-gated) |
| GET | `/v1/internal/corpus/workers` | Active leases grouped by `lease_owner` |

Never expose these on `/v1/public/*`.

---

## Implementation plan (gated PRs)

**Gate 0 (before any corpus code PR):**

1. **PR 0** (docs only): apply wording fixes in this file; align `docs/live-first-pivot.md` (no runtime changes).
2. Sync local `master` with `origin/master` (brings migrations `000045`–`000049`).
3. Verify MCP: `make mcp-verify`; optional `MCP_PREFLIGHT_HOSTED=1 bash scripts/hosted-data-mcp-smoke.sh`.

### PR 0 — Docs / source-of-truth cleanup (no runtime code)

- Align [`docs/live-first-pivot.md`](../live-first-pivot.md) with this file (public graph vs corpus pipeline counts; remove false “corpus deleted” claims).
- Optional: add `AGENTS.md` task-router pointer to this requirements doc.

### PR 0A — Archaeology (read-only)

- Confirm `GOLD_PER_VOD_GQL_RPM` wiring into GQL throttle path.
- Decide `gold_vod_rate_limits`: implement vs delete reference.
- Document production caller map: `BackfillWorker` → `SyncHistoricalStream` → `fetchVODCommentsParallel`.
- **No behavior changes.**

**PR 0A output** must be a short audit note (e.g. `docs/agent-notes/corpus-pr0a-audit.md`) containing:

- Whether `GOLD_PER_VOD_GQL_RPM` is wired end-to-end.
- Whether `gold_vod_rate_limits` should be removed or implemented (with rationale).
- Exact call path for Gold production execution.
- Exact tests that must be promoted to CI/nightly.
- Confirmation that local `master` matches `origin/master` for `migrations/`, `internal/`, `cmd/`.

### PR 0B — Resurrect Gold durable segments

PR 0B may be split if the diff grows beyond reviewable size. **Do not** combine store resurrection, completion semantics, and admin/API work in one PR.

Suggested split if needed:

| Sub-PR | Scope |
|--------|--------|
| **PR 0B-1** | Migration verification + store tests + CI/nightly integration gate |
| **PR 0B-2** | Production wiring for `Upsert` / `Claim` / `Complete` / `Fail` |
| **PR 0B-3** | Completion hook prevents false `done` |

**Scope (full PR 0B or split):**

- Ensure migration `000049` applied on BearHost Postgres (via normal migrate path).
- Wire segment store methods into Gold fetch lifecycle **additively** (keep checkpoints + in-memory queue).
- Enable `INTEGRATION=1` Gold segment tests in CI or scheduled workflow.
- Acceptance: hosted MCP shows non-zero `gold_vod_segments` activity during a Gold job; worker crash + reclaim succeeds in integration test.

**Rollback plan (PR 0B):**

- Disable durable segment wiring by setting `GOLD_VOD_SEGMENTS_ENABLED=false` (default).
- **Keep migration applied** — do not drop `gold_vod_segments` on rollback (migrations are forward-only).
- Existing checkpoint + in-memory Gold path must continue to work if segment ledger writing is disabled.

### PR 1 — Worker identity + scoped backoff

**Blocked until:** PR 0B deployed + one week of hosted segment failure metrics.

- Env: `SCRAPER_ID`, `GQL_WORKER_ID` (default host-derived, e.g. `streampulse-vps-0`).
- Extend only global TT backoff keys with identity suffix.
- Emit Prometheus labels by identity.

### PR 2 — Coverage ledger + gap detection

- Derive `top500_vod_inventory.gold_status` from segment ledger where possible.
- Known-empty vs missing classification.
- Internal gap list + requeue endpoint.

### PR 3 — Internal corpus detail API

- Endpoints listed above; auth same as existing internal analytics routes.

### PR 4 — Admin UI + Grafana

- Extend `deploy/grafana/dashboards/streamclone-ops.json`.
- Internal admin surface (StreamPulse web or backend frontend — **ADR required**, see open questions).

### PR 5 — Second worker identity rollout

**Blocked until:** PR 0B + PR 2 + hosted metrics review.

- Second host running same `analytics-workers` / scraper compose (PC burst or second VPS).
- **Not** laptop unless ADR reverses guardrail.

---

## Acceptance criteria (release checklist)

### Gate 0 / PR 0

- [ ] This doc and `live-first-pivot.md` consistent on public graph vs corpus pipeline counts.
- [x] PR 0A audit note published (`docs/agent-notes/corpus-pr0a-audit.md`); `gold_vod_rate_limits` → remove SELECT (PR 0B-1 `b3bc4b6`).

### PR 0B status (2026-07-01)

- [x] **0B-1 committed** (`b3bc4b6`): migrations `000045`–`000049`, segment rate-limit readiness, `make test-analytics-gold-segments`, nightly workflow.
- [x] **0B-2 committed** (`2bb6662` + audit fixes): `GOLD_VOD_SEGMENTS_ENABLED=false` default; durable ledger completes after rollup flush with per-segment `comments_fetched`.
- [x] **0B-3 committed** — completion gate on PR #31 (`feat/corpus-0b-gold-segments`).
- [ ] **0B stack deployed / hosted canary** — see `docs/agent-notes/corpus-0b2-hosted-verify.md` and `docs/agent-notes/corpus-hosted-baseline-2026-07-01.md`.

### PR 0B+ (acceptance)

- [ ] `origin/master` synced; migration `000049` present locally and applied on prod.
- [x] Hosted MCP smoke passes; corpus queue snapshots documented baseline (`docs/agent-notes/corpus-hosted-baseline-2026-07-01.md`).
- [ ] `gold_vod_segments` populated during Gold jobs; claim/complete/fail/reclaim proven.
- [x] Gold job cannot mark `done` with unresolved segment failures (PR 0B-3 completion gate; flag-gated).
- [ ] Public hub graph source semantics tested: live graph does not silently include `chat_source=gql|ivr`.
- [ ] Internal coverage API exposes stream/VOD/segment operational metadata only under internal auth.
- [ ] Known-empty requires successful fetch evidence, not merely missing rollup rows.
- [x] Public `/v1/public/hub` unchanged in sensitivity (aggregate-only; hosted probe 2026-07-01 + `TestPublicHubResponseOmitsSensitiveKeys`).
- [ ] Rollup dedupe tests pass; no duplicate minute rows from parallel segments.
- [ ] No corpus workloads on laptopworker compose profiles.
- [ ] Segment ledger disable flag (if added) verified: checkpoint + in-memory path still works.

---

## Risks

| Risk | Mitigation |
|------|------------|
| Doc-driven deletion of corpus code | This file + pivot doc fix + code review checklist |
| Migration number collision | Sync upstream before new migrations |
| False “done” Gold jobs | Segment ledger + completion hook |
| `gold_vod_rate_limits` confusion | Explicit design or delete reference in PR 0A |
| Premature multi-worker | Gate PR 1/5 on metrics |
| MCP misuse (writes) | `app_readonly` + client SELECT guard; no requeue tool |

---

## Open questions / ADRs

| ID | Question | Owner |
|----|----------|-------|
| OQ-1 | Admin corpus UI: `streampulse-web` internal route vs Streamclone `frontend/` vs Grafana-only? | Product |
| OQ-2 | Minimum Gold coverage % before portal shows “synced” for imported/corpus sessions? | Product |
| OQ-3 | `gold_vod_rate_limits`: Postgres table vs metrics-only? | Backend |
| OQ-4 | First second identity: home PC burst vs second VPS always-on? | Ops |
| OQ-5 | Retire or deploy `GoldIVRService` shadow/canary — timeline? | Backend |
| ADR-1 | Laptop corpus exception (if ever) — explicit written reversal of laptopworker guardrail | Ops |

---

## Agent routing

When touching corpus, Silver/Gold, backoff, or coverage:

1. Read **this file** first.
2. Read [`docs/live-first-pivot.md`](../live-first-pivot.md) for product boundaries.
3. Use **`streamclone-hosted-data`** MCP for prod queue/segment truth — never mutate via MCP.
4. Load skill `pulse/backfill-safety-review` before widening backfill or concurrency.
5. Do **not** implement PR 1+ until PR 0B acceptance criteria met.

Update this doc when PRs land (status section at top + checklist boxes).
