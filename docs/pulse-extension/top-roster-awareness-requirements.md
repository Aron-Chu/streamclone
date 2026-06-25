# Top-Roster Awareness (Plane A) — Requirements

| | |
|---|---|
| **Status** | Draft v1.2 — requirements (Plane A / B / C separation + BearHost mode matrix) |
| **Owner** | Aron-Chu |
| **Scope** | BearHost analytics (`streamclone`); extension/portal read-only honesty for new roster signals |
| **Batch ID** | **TOP-ROSTER-A** (awareness only — **not** top-500 IRC collection) |
| **Related** | [`multi-user-infra-review.md`](multi-user-infra-review.md) · [`bearhost-tunnel.md`](bearhost-tunnel.md) · [`../scraping-archive/requirements.md`](../scraping-archive/requirements.md) · sibling [`live-coverage-requirements.md`](../../streamclone-pulse/docs/pulse-extension/live-coverage-requirements.md) · [`roadmapping.md`](../roadmapping.md) |
| **Verdict** | **GO** TOP-ROSTER-A Phases 1–3 · **HOLD** admission · **HOLD** cap raise · **HOLD** ARCHIVE-SCHEDULER-A · **NO** top-500 IRC |
| **Repos** | Backend: **streamclone**. Extension parity: **streamclone-pulse** (coverage copy only; no new client logic required for Phase A). |

---

## 1. Problem statement

Streamclone has two workloads that must stay separated:

```text
Streamclone Archive  = durable historical corpus (GQL VOD chat, scraper, bronze/silver/gold)
Extension Pulse      = low-latency live product surface (IRC rollups, BFF cache, coverage)
```

Late joins on long streams still pay a **multi-minute reconstruction tax** when live rollups were never captured from go-live. R0/R1 built the **framework** for hosted multi-user Pulse (shared collector, quotas, protected go-live poller code, priority/preemption). What is missing is **Plane A**: cheap top-N roster **awareness** (who is live, stream IDs, viewer samples) without treating “top 500” as “500 IRC joins.”

This batch delivers **Top-Roster Awareness Only**:

- Discover and persist live/offline state for metadata top-N channels.
- Optionally admit **cap-aware** IRC collection at `TrackPriorityTopRoster` when explicitly enabled.
- Never join IRC for all top-N by default.
- Never trigger archive/backfill from awareness loops.

### What this batch does and does not do

| Outcome | Awareness-only (default) | With admission (HOLD on BearHost) |
|---------|--------------------------|-----------------------------------|
| Know who in top-N is live | ✅ Immediately | ✅ |
| Know `stream_id`, viewers, started_at | ✅ | ✅ |
| Honest “live but not collecting chat” UX | ✅ | ✅ |
| Live chat rollups during stream | ❌ | ✅ only if cap allows + admitted |
| Eliminate 10-minute post-hoc load | ❌ alone | ✅ only for admitted channels from go-live |
| Replace Bronze/Silver/Gold archive | ❌ never | ❌ never |

**Speed rule:** awareness improves **decision-making and UX honesty** immediately. **Admission** (Plane B from go-live) is what removes the reconstruction tax for channels that enter the collector early enough.

---

## 2. Goals and non-goals

### Goals

| ID | Goal |
|----|------|
| G-1 | Project metadata top-N (`tracked_streamers`) into `pulse_roster_state` with `source=top_roster`, priority `TrackPriorityTopRoster` (10). |
| G-2 | Run a batched Helix live-awareness worker that updates live/offline, `stream_id`, viewer count, `last_live_seen_at`, and go-live transitions. |
| G-3 | Emit observability for awareness vs admission vs capacity skips. |
| G-4 | Preserve existing Plane B cap (`PULSE_MAX_ACTIVE_CHANNELS`, default **10** on BearHost). |
| G-5 | Ensure protected / always-track / manual watch **always outrank** top-roster for collector admission. |
| G-6 | Keep extension/portal payloads **honest** about tracking vs awareness vs capacity. |

### Non-goals (explicit)

| ID | Non-goal |
|----|----------|
| NG-1 | Join IRC for all top-500 (or all live top-N) channels. |
| NG-2 | Store raw chat for top-roster channels without Plane B admission. |
| NG-3 | Trigger `SyncService` / GQL VOD backfill / corpus jobs from top-roster awareness. |
| NG-4 | Raise `PULSE_MAX_ACTIVE_CHANNELS` as part of this batch. |
| NG-5 | EventSub / webhook infrastructure (Helix batch poll only for MVP). |
| NG-6 | Replace `ProtectedGoLivePoller` — extend pattern, do not merge workers blindly. |

---

## 3. Architecture (three planes)

```text
Plane A — Discovery / top-roster awareness (THIS BATCH)
  tracked_streamers (Tier-0 metadata sync)
    → pulse_roster_state (source=top_roster, priority=10)
    → TopRosterAwarenessPoller (Helix batch, state only by default)
    → metrics + optional admin read API

Plane B — Capped live IRC collectors (EXISTING)
  collector.go + ircconn
    → in-memory minute accumulators → Postgres minute rollups
    → Redis BFF cache → /v1/extension/pulse/...

Plane C — Archive / backfill (OFF on BearHost Pulse mode)
  sync_gql_parallel, analytics-workers, scraper
    → MUST NOT be invoked from Plane A
```

See **§3.1** for how Plane A relates to Streamclone Archive (Bronze / Silver / Gold).

### Priority law (collector admission)

Existing constants in `internal/analytics/pulse_tracking_priority.go`:

| Priority | Tier | Source examples |
|---------:|------|-----------------|
| 80 | Global protected | `analytics_always_tracked` |
| 60 | Principal always-track | `pulse_watchlist.always_track=true` |
| 30 | Manual watch | Extension `POST /watch`, user-opened channel |
| 10 | Top roster | Metadata top-N (`source=top_roster`) |
| 0 | Idle / no ref | Evictable |

**Preemption:** `trackingPriorityCanPreempt` — top-roster (10) must **not** preempt manual watch (30+) or protected tiers.

### 3.1 Three-plane summary

```text
Plane A — Discovery / top-roster awareness (THIS BATCH, TOP-ROSTER-A)
  tracked_streamers → pulse_roster_state (source=top_roster)
  → TopRosterAwarenessPoller (Helix batch; no archive enqueue by default)

Plane B — Pulse live collector (EXISTING, capped)
  collector.go → minute rollups → Redis BFF → extension

Plane C — Streamclone Archive corpus (SEPARATE BearHost profile / host)
  Bronze → Silver → Gold via backfill_jobs, analytics-workers, scraper
```

**Canonical sentence:**

> Awareness tells Streamclone what exists and what was missed. Admission creates live rollups. Archive repairs missed history later.

See **§3.2–§3.7** for contracts, BearHost modes, archive backlog, and forbidden couplings.

### 3.2 Archive Coordination Contract

Plane A, Plane B, and Plane C share **read-only roster spine** (`tracked_streamers`) and may share Postgres / object storage on split hosts. They must **not** share process queues or enqueue paths.

#### Plane A may write (lightweight stream facts only)

| Field / signal | Storage |
|----------------|---------|
| `login`, `broadcaster_id` | `pulse_roster_state` |
| `stream_id` (`last_live_stream_id`) | `pulse_roster_state` |
| `stream_started_at` | `pulse_roster_state` |
| `viewer_count` (Helix snapshot) | `pulse_roster_state` |
| `is_live`, `last_live_seen_at`, `last_go_live_at` | `pulse_roster_state` |
| `reason_not_tracking`, collector admission status | `pulse_roster_state` |
| missed_start / partial coverage **hints** (adjunct only) | extension payload / admin — derived, not archive jobs |
| Go-live metrics | Prometheus (`pulse_golive_detected_total`, etc.) |

Plane A may update `tracked_streamers.last_seen_live_at` when Helix confirms live (same as Tier-0 / viewer sampler pattern).

#### Plane A must NOT write or enqueue

| Forbidden action | Owner / table (Plane C or extension repair) |
|------------------|---------------------------------------------|
| Silver tier jobs | `backfill_jobs` (`tier='silver'`) via `SilverEnqueuer`, `BackfillWorker` |
| Gold tier jobs | `backfill_jobs` (`tier` gold / gold_full / gold_lite) via `GoldEnqueuer`, `BackfillWorker` |
| GQL VOD chat corpus sync | `SyncService` (`sync_gql_parallel.go`) |
| Extension missed-moments backfill | `PulseBackfillManager` (Redis job state, `pulse_backfill.go`) |
| TwitchTracker / browser scrape | `scraper` compose service → silver worker path |
| Bronze export / archive job rows | `archive_jobs` (`000036_archive_jobs.up.sql`), `BronzeIndexer` |
| Scraper HTTP API calls | scraper internal API (`deploy` profile `scraper`) |

**Contract:**

| Plane | Produces | Consumes (later) |
|-------|----------|------------------|
| **A** | Awareness signals in `pulse_roster_state` | — |
| **B** | Live minute rollups for **admitted** channels only | Optional: A go-live for admission decision |
| **C** | Bronze/Silver/Gold artifacts | **Later:** A rows (stream_id, missed coverage) for **ARCHIVE-SCHEDULER-A** prioritization — not in TOP-ROSTER-A |

Plane C consumes awareness/coverage signals **only** from corpus profile (`bearhost-corpus-only.sh`) or a **separate worker host**, never from the Pulse API poller loop inline.

### 3.3 BearHost mode matrix

BearHost runs **one primary mode at a time** on the 8 GB VPS. Switch scripts: [`scripts/bearhost-pulse-api.sh`](../../scripts/bearhost-pulse-api.sh) vs [`scripts/bearhost-corpus-only.sh`](../../scripts/bearhost-corpus-only.sh). Preflight: [`scripts/bearhost-corpus-preflight.sh`](../../scripts/bearhost-corpus-preflight.sh).

| Capability | Pulse API mode (active today) | Corpus-only mode | Future split-host |
|------------|--------------------------------|------------------|-------------------|
| **Switch script** | `bearhost-pulse-api.sh` | `bearhost-corpus-only.sh` | Pulse host + worker host profiles |
| **Public extension API** | ✅ `pulse-caddy` :8090 → tunnel | ❌ off / degraded (no `pulse-caddy`) | ✅ Pulse host only |
| **Redis BFF cache** | ✅ | ❌ not priority | ✅ Pulse host |
| **Live IRC collector (Plane B)** | ✅ in `analytics`, cap 10 | ❌ off or not priority | ✅ Pulse host, capped |
| **Top-roster awareness (Plane A)** | ✅ allowed when flag on | ❌ not running | ✅ Pulse host |
| **Protected go-live** | gated (`PULSE_PROTECTED_GOLIVE_ENABLED`) | N/A | ✅ Pulse host |
| **Tier-0 roster sync** | ✅ `TIER0_ENABLED=true` in pulse profile | varies | either host with metadata |
| **`analytics-workers`** | ❌ stopped (compose profile `corpus`) | ✅ running | worker host |
| **`BackfillWorker` (Silver/Gold drain)** | ❌ | ✅ when `CORPUS_WORKERS_ENABLED=1` | worker host |
| **`scraper` (TwitchTracker)** | ❌ stopped | ✅ running | worker host |
| **Bronze indexer / Azure export** | ❌ `BRONZE_ENABLED=0` | ✅ after preflight | worker host |
| **Extension Pulse backfill API** | ✅ `POST /v1/extension/pulse/.../backfill` (Redis) | N/A | Pulse host |
| **Shared Postgres** | ✅ | ✅ | ✅ allowed |
| **Shared Azure blob** | optional read | ✅ write path | ✅ |
| **Process isolation** | corpus profile off | pulse-caddy / collector deprioritized | **required** |

**Purpose:**

| Mode | Primary goal |
|------|----------------|
| Pulse API | Beta extension, live product stability, shared collector + BFF |
| Corpus-only | Drain archive backlog, bronze/silver/gold, scraper health |
| Split-host | Run both without Q0 live path competing with corpus on one box |

Compose overlays: [`deploy/docker-compose.bearhost-pulse.yml`](../../deploy/docker-compose.bearhost-pulse.yml), [`deploy/env/profile-bearhost-pulse.env`](../../deploy/env/profile-bearhost-pulse.env), [`deploy/env/profile-bearhost-corpus.env`](../../deploy/env/profile-bearhost-corpus.env).

### 3.4 How TOP-ROSTER-A affects Silver / Gold

| Statement | True / false |
|-----------|--------------|
| TOP-ROSTER-A drains the current Silver/Gold queue | **False** |
| TOP-ROSTER-A enqueues new Silver/Gold jobs | **False** |
| TOP-ROSTER-A records stream IDs and live observations for future prioritization | **True** |
| Archive scheduler reads awareness rows later | **True** — **ARCHIVE-SCHEDULER-A** (out of scope here) |
| Extension auto-triggers archive on awareness | **False** |

Awareness makes **future** Silver/Gold prioritization smarter by persisting:

- which top-N streamers were live,
- which `stream_id` was active,
- viewer snapshots at observation time,
- whether Plane B collected chat (`tracking` / `reason_not_tracking`),
- implied missed-start when go-live was seen but no rollups exist.

**This document does not implement** the archive scheduler. It only ensures Plane A does not confuse agents into thinking awareness = queue draining.

Two backfill systems (do not conflate in docs or code):

| System | Storage | Trigger | BearHost Pulse mode |
|--------|---------|---------|---------------------|
| **Extension Pulse backfill** | Redis via `PulseBackfillManager` | Explicit `POST …/backfill` | ✅ available (quota-limited) |
| **Archive Silver/Gold** | Postgres `backfill_jobs` | `SilverEnqueuer`, `GoldEnqueuer`, `BackfillWorker` | ❌ workers stopped |

### 3.5 Current Silver / Gold backlog (BearHost Pulse mode)

When BearHost is in **Pulse API mode**, Silver/Gold jobs in Postgres are **paused**, not “stuck.”

**Observed snapshot (2026-06, Pulse mode, workers off):**

| tier | status | count (approx.) |
|------|--------|-----------------|
| silver | queued | 2,014 |
| silver | failed | 6,253 |
| silver | done | 1,323 |
| gold | done | 206 |
| gold | failed | 100 |
| running | — | **none** |

Most silver failures classify as stale **`scrape_backoff`** from earlier scraper/browser/TwitchTracker backoff — not active failures while the box is idle.

**Operator rules:**

- Queue counts **not changing** during TOP-ROSTER-A is **expected** in Pulse API mode.
- Resume corpus draining only via `bearhost-corpus-only.sh` or a **separate worker host** — not by enabling top-roster awareness.
- Before corpus mode: run [`scripts/bearhost-corpus-preflight.sh`](../../scripts/bearhost-corpus-preflight.sh) (Azure secret file, Twitch OAuth client creds) — see [`docs/bearhost-production.md`](../bearhost-production.md).
- **Do not** mass-retry thousands of stale `scrape_backoff` failures without classification (`scripts/bearhost-silver-gold-pg.sh` reason buckets).
- TOP-ROSTER-A smoke tests must assert **`backfill_jobs` row counts unchanged** across awareness ticks (S-30).

### 3.6 Forbidden couplings

| Coupling | Verdict |
|----------|---------|
| Awareness go-live → enqueue Gold / Silver (`backfill_jobs`) | **Forbidden** |
| Awareness go-live → TwitchTracker scrape | **Forbidden** |
| Awareness go-live → `SyncService` GQL VOD fetch | **Forbidden** |
| Awareness go-live → `PulseBackfillManager` | **Forbidden** |
| Extension page open → Silver/Gold enqueue | **Forbidden** |
| `awareness.live && !tracking` → `requestBackfill()` | **Forbidden** |
| Top-roster live candidate → preempt manual/protected collector | **Forbidden** |
| Top-roster awareness → raise `PULSE_MAX_ACTIVE_CHANNELS` | **Forbidden** |
| Awareness go-live → update `pulse_roster_state` | **Allowed** |
| Awareness go-live → Prometheus metrics | **Allowed** |
| Awareness go-live → `WatchWithPriority` | **Allowed only** if `PULSE_TOP_ROSTER_ADMISSION_ENABLED=true` **and** cap allows **and** priority allows (HOLD on BearHost) |
| Archive scheduler (future) reads awareness rows | **Allowed** (read-only, separate batch) |
| Extension shows “Live detected, chat tracking not active” | **Allowed** when adjunct public |

### 3.7 Implementation reference index (repo audit)

Verified paths in **streamclone** (2026-06). Use these names in implementation; do not guess.

#### Plane A / B (Pulse live — `analytics` container)

| Symbol | Path / artifact | Notes |
|--------|-----------------|-------|
| Tier-0 roster sync | [`internal/analytics/roster.go`](../../internal/analytics/roster.go) | `RosterSyncer`, `StartRosterWorker` |
| `tracked_streamers` | migration [`000032_tracked_streamers.up.sql`](../../migrations/000032_tracked_streamers.up.sql) | Shared roster spine |
| Viewer time-series samples | [`internal/analytics/viewer_sampler.go`](../../internal/analytics/viewer_sampler.go) | Plane B / Tier-0 live rows — not Plane A snapshot owner |
| `pulse_roster_state` | migration [`000042_pulse_roster_state.up.sql`](../../migrations/000042_pulse_roster_state.up.sql) | Plane A + protect rows |
| Roster store methods | [`internal/analytics/pulse_roster_state.go`](../../internal/analytics/pulse_roster_state.go) | `RefreshProtectedGoLiveRoster`, `UpdatePulseRosterPoll` |
| Protected go-live poller | [`internal/analytics/pulse_golive_poller.go`](../../internal/analytics/pulse_golive_poller.go) | `WatchWithPriority` when enabled |
| Top-roster awareness poller | **`internal/analytics/pulse_top_roster_poller.go`** | **Planned (TOP-ROSTER-A)** |
| IRC collector | [`internal/analytics/collector.go`](../../internal/analytics/collector.go) | Plane B |
| Priority / preemption | [`internal/analytics/pulse_tracking_priority.go`](../../internal/analytics/pulse_tracking_priority.go) | `TrackPriorityTopRoster = 10` |
| Coverage contract | [`internal/analytics/pulse_coverage.go`](../../internal/analytics/pulse_coverage.go) | Canonical `coverage.state` |
| Extension BFF | [`internal/analytics/extension_api.go`](../../internal/analytics/extension_api.go) | `/v1/extension/pulse/channels/{login}` |
| Extension Pulse backfill | [`internal/analytics/pulse_backfill.go`](../../internal/analytics/pulse_backfill.go) | Redis jobs — **not** `backfill_jobs` |
| Runtime flags | [`internal/analytics/pulse_runtime.go`](../../internal/analytics/pulse_runtime.go) | Awareness/admission env |
| Metrics | [`internal/metrics/pulse.go`](../../internal/metrics/pulse.go) | |
| Process wiring | [`cmd/analytics/main.go`](../../cmd/analytics/main.go) | Starts collector, protected poller, Tier-0 |

#### Plane C (Archive corpus — `analytics-workers` + `scraper`)

| Symbol | Path / artifact | Notes |
|--------|-----------------|-------|
| Silver/Gold queue table | [`backfill_jobs`](../../migrations/000033_backfill_jobs.up.sql) | `tier`, `status`, `error`, `next_run_at` |
| Archive job progress | [`archive_jobs`](../../migrations/000036_archive_jobs.up.sql) | Bronze / export plane |
| Silver job enqueuer | [`internal/analytics/silver_enqueuer.go`](../../internal/analytics/silver_enqueuer.go) | Inserts silver `backfill_jobs` |
| Gold job enqueuer | [`internal/analytics/gold_enqueuer.go`](../../internal/analytics/gold_enqueuer.go) | Inserts gold tier jobs |
| Queue worker | [`internal/analytics/backfill_worker.go`](../../internal/analytics/backfill_worker.go) | Runs in **`analytics-workers`** |
| GQL VOD chat sync | [`internal/analytics/sync_gql_parallel.go`](../../internal/analytics/sync_gql_parallel.go) | Gold / extension backfill path |
| Sync orchestration | `SyncService` in analytics package | Used by backfill worker + Pulse backfill |
| Queue status CLI | [`scripts/bearhost-silver-gold-pg.sh`](../../scripts/bearhost-silver-gold-pg.sh) | Failure reason buckets |

#### BearHost ops scripts

| Script | Purpose |
|--------|---------|
| [`scripts/bearhost-pulse-api.sh`](../../scripts/bearhost-pulse-api.sh) | Pulse API mode — stops `analytics-workers`, `scraper` |
| [`scripts/bearhost-corpus-only.sh`](../../scripts/bearhost-corpus-only.sh) | Corpus mode — starts workers + scraper |
| [`scripts/bearhost-corpus-preflight.sh`](../../scripts/bearhost-corpus-preflight.sh) | Azure + Twitch cred gate |
| [`deploy/smoke/bearhost-pulse-api.sh`](../../deploy/smoke/bearhost-pulse-api.sh) | Public health smoke |

#### Extension (streamclone-pulse)

| Symbol | Path |
|--------|------|
| Auto-track / page open | [`src/content/entry.ts`](../../streamclone-pulse/src/content/entry.ts) |
| Backfill UX | [`src/ui/Overlay.tsx`](../../streamclone-pulse/src/ui/Overlay.tsx), [`src/ui/missedMoments.ts`](../../streamclone-pulse/src/ui/missedMoments.ts) |

**Corrections from earlier drafts:** Extension repair uses **`PulseBackfillManager` (Redis)**, not Postgres `backfill_jobs`. Archive Silver/Gold uses **`backfill_jobs`** + **`analytics-workers`**, not the `analytics` Pulse API container.

---

## 4. Current repo audit (baseline)

Full path index: **§3.7**. Summary of TOP-ROSTER-A delivery gaps:

| Component | Status | See §3.7 |
|-----------|--------|----------|
| Tier-0 → `tracked_streamers` | ✅ Shipped | `roster.go` |
| `pulse_roster_state` | ✅ Table exists | migration `000042` |
| Protected go-live poller | ✅ Shipped, flag off | `pulse_golive_poller.go` |
| Top-roster awareness poller | ❌ Planned | `pulse_top_roster_poller.go` |
| Top-roster projection into roster state | ❌ Planned | `RefreshTopRosterAwarenessRoster` |
| Archive Silver/Gold drain | ⏸ Paused on BearHost Pulse mode | `backfill_jobs`, `analytics-workers` |
| Extension adjunct / copy keys | ❌ Planned | streamclone-pulse |
| `capacity_full`, `missed_start` adjunct UX | ❌ Planned | §8 |

---

## 5. Functional requirements

### 5.1 Roster projection (Tier-0 → pulse_roster_state)

| ID | Requirement |
|----|-------------|
| TRA-001 | Add `Store.RefreshTopRosterAwarenessRoster(ctx, topN int)` that upserts into `pulse_roster_state` from `tracked_streamers` ordered by `last_rank ASC NULLS LAST`, limited to `topN`. |
| TRA-002 | Insert/update uses `source='top_roster'`, `priority=TrackPriorityTopRoster` (10). |
| TRA-003 | On conflict, **never downgrade** priority or source if existing row has higher priority (same pattern as `RefreshProtectedGoLiveRoster`). |
| TRA-004 | `topN` defaults to `PULSE_ROSTER_SIZE` (500) but BearHost may override via env; Tier-0 sync may still write only top 200 — awareness uses **intersection** of configured cap and rows present. |
| TRA-005 | Projection runs at worker start and on a configurable interval (`PULSE_TOP_ROSTER_SYNC_INTERVAL`, default 15m). |
| TRA-006 | Projection **must not** call `Collector.Watch`, `SyncService`, `PulseBackfillManager`, `BackfillWorker`, `SilverEnqueuer`, `GoldEnqueuer`, scraper APIs, or insert into `backfill_jobs` / `archive_jobs` (see §3.2, §3.6). |

### 5.2 Top-roster awareness worker (Helix batch, default: no IRC)

| ID | Requirement |
|----|-------------|
| TRA-010 | Add `TopRosterAwarenessPoller` in new file `internal/analytics/pulse_top_roster_poller.go` (name may vary; keep distinct from protected poller). |
| TRA-011 | Worker starts only when `PULSE_TOP_ROSTER_AWARENESS_ENABLED=true` (new flag; distinct from admission flag). |
| TRA-012 | Each tick: `RefreshTopRosterAwarenessRoster` → `ListPulseRosterDue` filtered to `source='top_roster'` OR dedicated `ListTopRosterDue` query. |
| TRA-013 | Batch Helix `UsersByLogin` (missing broadcaster IDs) and `StreamsByLogin` (max `PULSE_GOLIVE_BATCH_SIZE`, default 100). |
| TRA-014 | Persist per login: `broadcaster_id`, `last_live_stream_id`, `last_live_seen_at`, `next_poll_after`, `last_error_code`, `is_live`, `viewer_count`, `stream_started_at`, `reason_not_tracking`. |
| TRA-014b | **Changed-row writes only:** do not rewrite all top-N rows every tick. Persist only when `is_live`, `last_live_stream_id`, `viewer_count`, `stream_started_at`, `reason_not_tracking`, `last_error_code`, or scheduling timestamps (`last_polled_at`, `next_poll_after`) **materially change**. Always update `last_polled_at` at most once per tick per row processed. |
| TRA-014c | **Viewer count ownership:** `TopRosterAwarenessPoller` stores the **latest Helix snapshot** in `pulse_roster_state.viewer_count`. `ViewerSampler` (`viewer_sampler.go`) remains the owner of **time-series viewer minute samples** into stream rollups for channels already in Plane B or Tier-0 live tracking. Do **not** duplicate long-horizon viewer sample writes from both paths for the same minute unless Plane B is active for that login. |
| TRA-015 | Detect offline→live transition when `current_stream_id != last_live_stream_id` and live with non-empty `current_stream_id`. |
| TRA-015b | **Stale-live cleanup:** if Helix confirms offline, set `is_live=false` and clear `reason_not_tracking` unless admission-disabled/capacity reason should persist for UX. On Helix **429/5xx**, do **not** immediately mark offline — preserve previous live snapshot, set `last_error_code`, backoff `next_poll_after`. If a row was live and misses **N consecutive successful polls** without live confirmation (`PULSE_TOP_ROSTER_STALE_POLLS`, default 3), mark `is_live=false`. |
| TRA-016 | On go-live detection: call `Collector.NoteGoLiveDetected(streamID, login, "top_roster", TrackPriorityTopRoster, duplicate)` for metrics **only** unless admission enabled (TRA-020). |
| TRA-017 | Update `tracked_streamers.last_seen_live_at` when live (reuse store helper or shared upsert used by viewer sampler). |
| TRA-018 | Worker respects Helix rate limits: backoff on 429, stagger `next_poll_after` per row. |
| TRA-019 | Worker logs structured fields: `login`, `stream_id`, `live`, `viewer_count`, `go_live`, `admitted`, `skip_reason`. |

### 5.3 Optional cap-aware admission (explicit opt-in)

| ID | Requirement |
|----|-------------|
| TRA-020 | IRC admission requires **`PULSE_TOP_ROSTER_ADMISSION_ENABLED=true`** (separate from awareness). Default **false** on BearHost. **Implement admission code in Phase 4; do not enable on BearHost until G2+G3 (§12).** |
| TRA-021 | Admission effective when `PULSE_TOP_ROSTER_ADMISSION_ENABLED=true` only. `PULSE_TOP_ROSTER_POLL_ENABLED` is a **legacy alias** (deprecated); if set without `ADMISSION_ENABLED`, log a warning and treat as no-op. Do not require three flags long-term. |
| TRA-022 | Before `WatchWithPriority`, verify `collector.ActiveCount() < PULSE_MAX_ACTIVE_CHANNELS`. |
| TRA-023 | Call `WatchWithPriority(ctx, login, "", TrackPriorityTopRoster)` only on first go-live for `stream_id`; skip duplicate stream observations (mirror protected poller). |
| TRA-024 | If cap full, record `reason_not_tracking=capacity_full` on roster row (new column — see §7) and increment `pulse_top_roster_skipped_capacity_total`. |
| TRA-025 | If incoming channel would require preempting priority ≥ manual watch, **do not preempt**; record `reason_not_tracking=lower_priority` (or `protected_priority`). |
| TRA-026 | Admission must not starve Q0: protected go-live poller retains higher scheduling priority in the same process. |

### 5.4 Extension / BFF honesty (read path)

| ID | Requirement |
|----|-------------|
| TRA-030 | Extension pulse payload continues to use backend `coverage` as canonical (`internal/analytics/pulse_coverage.go`). |
| TRA-031 | Add **`rosterAwareness`** adjunct on pulse payload — **admin-first rollout** (see §13). Prefer adjunct over new primary `coverage.state` values to avoid breaking contract tests. |
| TRA-031b | **Extension exposure gated:** Phase 2 = admin health + internal API only; Phase 3 = pulse payload adjunct behind `PULSE_ROSTER_AWARENESS_PUBLIC=true` (default false); Phase 4 = extension copy keys + UI. |
| TRA-038 | **Awareness live ≠ chat collected:** when `tracking=false` and adjunct shows live, extension **must not** show chat velocity, top emotes, most-reacted, or heatmap confidence unless live rollups exist. Allowed: viewer snapshot, “Live detected”, “Chat tracking not active”, reason string. |
| TRA-039 | **Anti-pattern (forbidden):** `if awareness.live && !tracking { requestBackfill() }` in extension or BFF. Regression test required. |
| TRA-032 | When channel is in top-roster awareness but **not** in Plane B collector, surface `tracking: false` and adjunct `awarenessState: "live_not_collected"` (or `copyKey: not_tracking`). |
| TRA-033 | When collector active: `tracking: true` (unchanged). |
| TRA-034 | When cap blocked top-roster admission: adjunct `collectorAdmission: "capacity_full"` — **do not** imply backfill availability. |
| TRA-035 | When live rollups exist but `coverageStartOffsetSeconds > tolerance`: existing `partial_tracking` / `missing_ranges_detected` — map UX copy to **missed_start** in portal/extension copy keys (may alias existing states; no fake `full_stream_tracked`). |
| TRA-036 | Top-roster awareness **must not** set `canBackfill=true` or enqueue backfill. |
| TRA-037 | Extension client (`streamclone-pulse`) must not auto-backfill on awareness alone — existing `canBackfill` gate remains (regression test). |

### 5.5 Observability

| ID | Requirement |
|----|-------------|
| TRA-040 | Prometheus gauges/counters in `internal/metrics/pulse.go`: |
| | `pulse_top_roster_live_candidates` — count of top-roster rows live this tick |
| | `pulse_top_roster_admitted_collectors` — cumulative admissions this process |
| | `pulse_top_roster_skipped_capacity_total` — cap-full skips |
| | `pulse_top_roster_skipped_priority_total` — would-preempt-higher-priority skips |
| | Reuse `pulse_golive_detected_total{source="top_roster"}` |
| | Reuse `pulse_golive_to_first_rollup_seconds` when admission enabled |
| TRA-041 | Extend `RefreshPulseMetricGauges` / admin health (`pulse_admin_health.go`) with top-roster flag states. |
| TRA-042 | Log sample rate: 100% on go-live transitions; debug on routine polls. |

### 5.6 Kill switches and env

| ID | Requirement |
|----|-------------|
| TRA-050 | `PULSE_TOP_ROSTER_AWARENESS_ENABLED` — master switch for Plane A worker (default **false**). |
| TRA-051 | `PULSE_TOP_ROSTER_ADMISSION_ENABLED` — allows IRC join on go-live (default **false**). |
| TRA-052 | `PULSE_TOP_ROSTER_POLL_ENABLED` — **legacy alias only** for `ADMISSION_ENABLED`; default **false**; prefer not to set on BearHost. |
| TRA-053 | `PULSE_TOP_ROSTER_SYNC_INTERVAL` — roster projection refresh (default `15m`). |
| TRA-054 | `PULSE_TOP_ROSTER_INTERVAL` — Helix poll interval (default `3m`; ~5 batched `StreamsByLogin` calls per tick at batch 100). |
| TRA-055 | `PULSE_ROSTER_SIZE` — max top-N rows to project (default 500). |
| TRA-056 | Disabling awareness stops worker on next tick; does not Part() existing collectors. |
| TRA-057 | `PULSE_TOP_ROSTER_STALE_POLLS` — consecutive missed live confirmations before `is_live=false` (default `3`). |
| TRA-058 | `PULSE_ROSTER_AWARENESS_PUBLIC` — expose adjunct on public extension pulse payload (default **false**). |

---

## 6. File-by-file implementation plan

### 6.1 Backend (streamclone) — new / modified

| File | Action |
|------|--------|
| `docs/pulse-extension/top-roster-awareness-requirements.md` | **This document** |
| `internal/analytics/pulse_top_roster_poller.go` | **Add** — awareness worker + optional admission |
| `internal/analytics/pulse_roster_state.go` | **Extend** — `RefreshTopRosterAwarenessRoster`, `ListTopRosterDue`, admission reason updates |
| `internal/analytics/pulse_runtime.go` | **Extend** — new env vars, defaults |
| `internal/analytics/pulse_tracking_priority.go` | **Verify** — tests for top-roster vs manual/preempt |
| `internal/analytics/pulse_observability.go` | **Verify** — `top_roster` source class already normalized |
| `internal/metrics/pulse.go` | **Extend** — new metrics |
| `internal/analytics/pulse_admin_health.go` | **Extend** — expose awareness/admission flags |
| `internal/analytics/extension_api.go` | **Extend** — optional `rosterAwareness` adjunct on pulse payload |
| `internal/analytics/pulse_coverage.go` | **Optional** — copy keys only; avoid new primary states unless necessary |
| `cmd/analytics/main.go` | **Wire** — `StartTopRosterAwarenessPoller(...)` after protected poller |
| `deploy/smoke/top-roster-awareness.sh` | **Add** — S-10–S-14, S-25, S-30–S-31 helpers |
| `deploy/env/profile-bearhost-pulse.env` | **Document** — flags default false |
| `.env.example` | **Document** — new env vars |
| `docs/pulse-extension/multi-user-infra-review.md` | **Cross-link** — one paragraph pointing to this batch |
| `docs/roadmapping.md` | **Cross-link** — Phase 4 top-roster awareness |

### 6.2 Migrations

| Migration | Purpose |
|-----------|---------|
| `000043_pulse_roster_awareness.up.sql` | Add columns to `pulse_roster_state`: `is_live BOOL DEFAULT false`, `viewer_count INT`, `stream_started_at TIMESTAMPTZ`, `reason_not_tracking TEXT DEFAULT ''`, `last_go_live_at TIMESTAMPTZ`. Index: `(source, is_live)` partial where `source='top_roster'`. |
| `000043_pulse_roster_awareness.down.sql` | Drop added columns/index. |

**Alternative (if avoiding migration in v1):** pack transient viewer count into Redis keyed by login with TTL; persist only stream_id transitions in Postgres. **Recommendation:** migration is cleaner for admin/debug and matches requirements.

### 6.3 Extension (streamclone-pulse) — minimal

| File | Action |
|------|--------|
| `src/shared/messages.ts` | **Extend** — optional `rosterAwareness` / `collectorAdmission` types |
| `src/ui/missedMoments.ts` | **Optional** — copy keys for `not_tracking`, `capacity_full`, `missed_start` adjunct |
| `tests/missedMoments.test.ts` | **Extend** — fixtures for new adjunct fields |
| `docs/pulse-extension/live-coverage-requirements.md` | **Cross-link** §11 top roster |

No extension worker changes required for Plane A awareness-only deploy.

---

## 7. Data model (post-migration)

### `pulse_roster_state` (extended)

| Column | Type | Notes |
|--------|------|-------|
| `login` | TEXT PK | normalized lower |
| `source` | TEXT | `top_roster`, `global_protected`, `principal_always_track` |
| `priority` | INT | 10 / 60 / 80 |
| `broadcaster_id` | TEXT | Helix cache |
| `last_live_stream_id` | TEXT | dedupe go-live |
| `last_live_seen_at` | TIMESTAMPTZ | last Helix live observation |
| `is_live` | BOOL | current snapshot |
| `viewer_count` | INT | last Helix viewer count when live |
| `stream_started_at` | TIMESTAMPTZ | Helix `started_at` when live |
| `reason_not_tracking` | TEXT | `''`, `capacity_full`, `lower_priority`, `admission_disabled` |
| `last_go_live_at` | TIMESTAMPTZ | last offline→live transition |
| `last_polled_at` / `next_poll_after` | TIMESTAMPTZ | scheduling |
| `last_error_code` | TEXT | helix errors |

### `tracked_streamers` (unchanged)

Tier-0 sync continues via `RosterSyncer.SyncOnce` — awareness reads, does not replace.

---

## 8. Coverage and UX honesty mapping

Backend primary coverage states remain in `pulse_coverage.go`. Top-roster awareness adds **adjunct** signals:

| User-visible concept | Backend signal | Notes |
|---------------------|----------------|-------|
| **tracking** | `tracking: true` + rollups | Plane B active |
| **not_tracking** | `tracking: false`, live in awareness | `rosterAwareness.live=true`, `collectorAdmission=admission_disabled` |
| **capacity_full** | cap blocked admission | `reason_not_tracking=capacity_full` |
| **partial_live** | rollups + `coverageStartOffsetSeconds > 60` | existing `partial_tracking` |
| **missed_start** | live + no rollups from T+0 | `partial_tracking` + copyKey alias or `missed_start` adjunct |

**Do not** conflate awareness live with chat collection in UI badges.

### Extension copy guidelines (when adjunct is public)

| Show | When |
|------|------|
| “Live detected” / viewer count | `rosterAwareness.live=true` |
| “Chat tracking not active” | `tracking=false` + live in awareness |
| “Capacity full — not collecting” | `collectorAdmission=capacity_full` |
| “Tracking” / chat stats / emote lanes | `tracking=true` + rollups present only |

Avoid “Monitoring”, “Full stream data”, or green TRACKING badge unless Plane B is active.

---

## 8.1 Risk areas (review sign-off)

| Risk | Mitigation |
|------|------------|
| Top-roster competes with manual users | Priority 30 vs 10; smoke S-25; never preempt manual/protected |
| Awareness triggers auto-backfill | TRA-036, TRA-039, NG-3; extension regression test |
| Users assume chat tracked from viewer count | TRA-038, copy guidelines above |
| Admission enabled too early | HOLD on BearHost; code in Phase 4, flags off |
| Postgres churn from 500-row rewrites | TRA-014b changed-row writes only |
| False offline on Helix blip | TRA-015b stale-live + error backoff |
| Double viewer sample writes | TRA-014c ownership split vs `ViewerSampler` |

---

## 9. Test plan

### 9.1 Go unit tests (`streamclone`)

| Test file | Cases |
|-----------|-------|
| `internal/analytics/pulse_top_roster_poller_test.go` | Awareness tick updates state; **no** `WatchWithPriority` when admission disabled |
| | Go-live transition updates `last_live_stream_id` |
| | Unchanged row → no store write (TRA-014b) |
| | Helix 429 → previous `is_live` preserved, error set |
| | Stale poll threshold → `is_live=false` |
| | Duplicate stream_id does not double-admit |
| | Cap full → `reason_not_tracking=capacity_full`, skip counter |
| | Manual/protected priority would be preempted → skip admission |
| | **No** call to `PulseBackfillManager` / `SyncService` / enqueuers |
| `internal/analytics/pulse_top_roster_admission_test.go` | **S-25:** cap=2, two manual watches active, top-roster go-live → no admission |
| `internal/analytics/pulse_roster_state_test.go` | Top-roster projection from `tracked_streamers` |
| | Higher-priority row not downgraded on sync |
| `internal/analytics/pulse_tracking_priority_test.go` | Top-roster cannot preempt manual watch |
| `internal/analytics/pulse_security_test.go` | (existing) rate limits unaffected |

Run:

```bash
go test ./internal/analytics/... -run 'TopRoster|RosterAwareness|GoLive|TrackingPriority'
```

### 9.2 Extension tests (`streamclone-pulse`)

| Test file | Cases |
|-----------|-------|
| `tests/missedMoments.test.ts` | Payload with `tracking:false` + awareness adjunct does not enable load CTA |
| | Payload with `awareness.live && !tracking` does not trigger backfill helper paths (TRA-039) |
| `tests/coverageFixtures.test.ts` | Fixture: top-roster live, not collected, honest copy |
| `tests/rosterAwareness.test.ts` (new) | UI hides chat stats when tracking false despite live awareness |

Run:

```bash
cd streamclone-pulse && npm test -- missedMoments coverageFixtures
```

### 9.3 Contract / drift

| Check | Command |
|-------|---------|
| API contract keys | `.cursor/skills/pulse/api-contract-drift-check` script on extension pulse types vs sample payload |

---

## 10. Smoke checklist (BearHost)

Run **before** and **after** enabling flags.

### 10.1 Baseline (flags off)

```bash
PULSE_SMOKE_BASE_URL=https://api.streampulse.stream \
  PULSE_EXPECT_HOSTED_MODE=true \
  ./deploy/smoke/bearhost-pulse-api.sh
```

| # | Check | Pass criteria |
|---|-------|---------------|
| S-1 | Public health | `hostedMode=true`, `helixEnabled=true`, routes present |
| S-2 | Active collector cap | `activeTracked <= 10` under normal beta use |
| S-3 | Protected poller | `protectedGoLiveEnabled=false` (until separate R1 gate) |
| S-4 | Top-roster flags | `topRosterPollEnabled=false`, awareness off in admin health |

### 10.2 Awareness only (`PULSE_TOP_ROSTER_AWARENESS_ENABLED=true`, admission **false**)

| # | Check | Pass criteria |
|---|-------|---------------|
| S-10 | Roster projection | `pulse_roster_state` rows with `source=top_roster` appear after Tier-0 sync |
| S-11 | Live metadata | Known live top streamer row updates `is_live`, `viewer_count`, `last_live_stream_id` |
| S-12 | **No IRC spike** | `pulse_active_tracked_channels` unchanged vs baseline when no user watches |
| S-13 | No backfill | `pulse_backfill_active_jobs` not incremented by poller |
| S-14 | Metrics | `pulse_top_roster_live_candidates` > 0 during peak hours |

### 10.3 Admission opt-in (only after S-10–S-14 stable 24h)

Enable `PULSE_TOP_ROSTER_ADMISSION_ENABLED=true` on staging or limited window.

| # | Check | Pass criteria |
|---|-------|---------------|
| S-20 | Cap respected | `activeTracked <= PULSE_MAX_ACTIVE_CHANNELS` always |
| S-21 | Priority | Protected/manual watches still preempt top-roster under load test |
| S-22 | Go-live metric | `pulse_golive_to_first_rollup_seconds{source=top_roster}` observed when admitted |
| S-23 | Skip honesty | Cap-full candidates have `reason_not_tracking=capacity_full` |

### 10.4 Protected go-live regression

When `PULSE_PROTECTED_GOLIVE_ENABLED=true` (separate R1 gate), re-run S-21 to confirm protected still outranks top-roster.

### 10.5 Manual vs top-roster priority (S-25)

| # | Check | Pass criteria |
|---|-------|---------------|
| S-25 | Cap contention | With `PULSE_MAX_ACTIVE_CHANNELS=2` and two manual watches active, inject top-roster go-live → **no** third IRC join; `reason_not_tracking` ∈ (`capacity_full`, `lower_priority`) |

### 10.6 Archive isolation (corpus host or flags)

| # | Check | Pass criteria |
|---|-------|---------------|
| S-30 | No Silver/Gold enqueue | `SELECT COUNT(*) FROM backfill_jobs` unchanged across awareness tick window |
| S-31 | No Pulse extension backfill | `pulse_backfill_active_jobs` metric unchanged unless explicit `POST …/backfill` |
| S-32 | No scraper activity | No new scraper container jobs / TT scrape requests from awareness |

---

## 11. Rollback plan

| Step | Action |
|------|--------|
| R-1 | Set `PULSE_TOP_ROSTER_AWARENESS_ENABLED=false` and `PULSE_TOP_ROSTER_ADMISSION_ENABLED=false` in BearHost env |
| R-2 | `docker compose up -d --force-recreate analytics` (pulse redeploy script) |
| R-3 | Verify S-1, S-2, S-12 |
| R-4 | Migration `000043` is forward-only — rollback **does not** require down migration; new columns ignored when worker off |
| R-5 | If admission caused IRC over-cap (should not happen), restart analytics container to reset in-memory collector state |

No data loss: awareness rows in `pulse_roster_state` are safe to leave stale.

---

## 12. Go / no-go gates (BearHost)

| Gate | Awareness only | + Admission |
|------|----------------|-------------|
| **G0** Deploy smoke PASS | Required | Required |
| **G1** R0-b quotas live | Required | Required |
| **G2** 24h soak @ cap 10 (Batch R) | Recommended | **Required** |
| **G3** Protected go-live soak | Not required | Recommended before admission |
| **G4** Synthetic load LOAD-001 | Not required | Recommended |
| **Verdict** | **GO** after G0+G1 | **NO GO** until G2+G3 |

### Summary verdict (from feasibility review)

| Option | Recommendation |
|--------|----------------|
| A) Small extension beta | **GO** (unchanged) |
| B) Protected go-live @ cap 10 | **GO** after smoke + soak (separate flag) |
| C) Top-500 **awareness only** | **GO** after G0+G1 — this batch |
| D) Top-500 IRC for all live | **NO GO** |

---

## 13. Phased delivery (implementation order)

**BearHost default through Phase 3:** awareness only; admission code may land disabled.

| Phase | Deliverable | Flags / exposure |
|-------|-------------|----------------|
| **1** | Migration `000043`, store methods, projection | none |
| **2** | Awareness poller (no IRC), metrics, **admin health only** | `PULSE_TOP_ROSTER_AWARENESS_ENABLED=true` |
| **3** | Smoke script `deploy/smoke/top-roster-awareness.sh`; S-10–S-14, S-30–S-32 | awareness only |
| **4** | Admission path + priority/cap tests (S-25); **code only, flags off on BearHost** | `ADMISSION_ENABLED=false` in prod |
| **5** | Extension types + copy; pulse adjunct behind public flag | `PULSE_ROSTER_AWARENESS_PUBLIC=true` when copy ready |
| **6** | Protected go-live soak (separate R1 batch) | `PULSE_PROTECTED_GOLIVE_ENABLED` |
| **7** | Top-roster admission window (staging → limited prod) | after G2+G3 |

Estimated: **1–2 backend sessions** for Phases 1–3; Phases 4–7 gated on Batch Q/R soak.

### Approved for implementation now

```text
GO:   TOP-ROSTER-A Phases 1–3 (awareness, admin, metrics, smoke)
HOLD: top-roster admission on BearHost
HOLD: cap increase (10 → 25)
HOLD: ARCHIVE-SCHEDULER-A (separate requirements doc)
NO:   top-500 IRC collection
NO:   raw chat storage for awareness-only channels
NO:   auto Silver/Gold/backfill from extension page open or awareness ticks
```

---

## 14. Open questions

| ID | Question | Default / owner |
|----|----------|-----------------|
| OQ-1 | Filter `ListPulseRosterDue` by source vs separate query? | Separate `ListTopRosterDue` |
| OQ-2 | Public extension adjunct timing? | **Resolved:** admin-first; `PULSE_ROSTER_AWARENESS_PUBLIC` |
| OQ-3 | Sync interval vs Tier-0 roster interval? | Awareness poll 3m; Tier-0 unchanged |
| OQ-4 | EventSub later? | Out of scope; hook in poller interface |
| OQ-5 | Deprecate `PULSE_TOP_ROSTER_POLL_ENABLED`? | Legacy alias for admission only |
| OQ-6 | **ARCHIVE-SCHEDULER-A** doc owner & P2 viewer threshold? | Separate requirements doc; not TOP-ROSTER-A |
| OQ-7 | Split-host timing vs single VPS? | Defer until cap-10 soak + corpus preflight clean |
| OQ-8 | Admin UI for backlog vs awareness? | Read-only join on `stream_id`; ARCHIVE-SCHEDULER-A |

---

## 15. Revision history

| Date | Change |
|------|--------|
| 2026-06-24 | Initial requirements from feasibility review (Plane A only) |
| 2026-06-24 | v1.1 — review tightenings: changed-row writes, viewer ownership, stale-live, flag semantics, admin-first extension, risks, S-25/S-30 |
| 2026-06-24 | v1.2 — Archive Coordination Contract, BearHost mode matrix, Silver/Gold backlog note, forbidden couplings, §3.7 repo index, ARCHIVE-SCHEDULER-A, acceptance criteria AC-A/C/S |

---

## 16. Future batch: ARCHIVE-SCHEDULER-A (out of scope)

**Not part of TOP-ROSTER-A.** Separate requirements doc (`docs/pulse-extension/archive-scheduler-requirements.md` — TBD) should define:

| Topic | Scope |
|-------|--------|
| Silver/Gold job prioritization | Read `pulse_roster_state`, coverage, protect flags |
| Archive candidate selection | Which streams become repair targets |
| Explicit user backfill precedence | Extension `POST …/backfill` / portal actions outrank passive top-roster archive |
| Stale `scrape_backoff` retry policy | Classify before bulk retry; no mass unbounded retry |
| Host/profile gate | Only corpus profile or worker host may enqueue/drain |
| Extension archive status UX | Show backfill/archive state **without** auto-triggering |

**Suggested archive priority order (draft for ARCHIVE-SCHEDULER-A):**

| Priority | Candidate |
|----------|-----------|
| **P0** | Protected / always-track stream with missed live coverage |
| **P1** | Explicit user-requested backfill (extension or portal) |
| **P2** | Recent top-roster stream above viewer threshold |
| **P3** | Top-roster stream where Plane B missed start |
| **P4** | Normal recent top-N archive |
| **P5** | Stale failed `scrape_backoff` retries (selective, classified) |

ARCHIVE-SCHEDULER-A may **read** Plane A rows; it must not run inside the Pulse API poller or extension page-load path.

---

## 17. Acceptance criteria (plane separation)

### 17.1 Awareness-only mode (TOP-ROSTER-A Phases 1–3)

| # | Criterion | Pass |
|---|-----------|------|
| AC-A1 | Top-roster rows in `pulse_roster_state` with `source=top_roster` | After projection + poll |
| AC-A2 | Live metadata updates for known live top-N channels | `is_live`, `viewer_count`, `last_live_stream_id` |
| AC-A3 | Active IRC count unchanged from awareness alone | `pulse_active_tracked_channels` flat without user watches |
| AC-A4 | `backfill_jobs` counts unchanged across awareness ticks | S-30 SQL snapshot |
| AC-A5 | No scraper jobs created | S-32 |
| AC-A6 | No Pulse extension backfill jobs created | S-31 |
| AC-A7 | Extension hides chat stats when `tracking=false` | TRA-038, extension tests |
| AC-A8 | No `canBackfill=true` from awareness path | TRA-036 |

### 17.2 Corpus mode (separate operator action)

| # | Criterion | Pass |
|---|-----------|------|
| AC-C1 | Silver/Gold workers drain only when corpus profile active | `analytics-workers` up, `CORPUS_WORKERS_ENABLED=1` |
| AC-C2 | Stale `scrape_backoff` not bulk-retried by default | Operator playbook + scheduler doc |
| AC-C3 | Archive workers do not require top-roster admission | `BackfillWorker` independent of Plane B IRC |

### 17.3 Split-host future

| # | Criterion | Pass |
|---|-----------|------|
| AC-S1 | Pulse host runs awareness + capped collector | Pulse profile |
| AC-S2 | Worker host drains corpus | Corpus profile |
| AC-S3 | Shared DB/blob without shared in-process queues | No cross-container enqueue |
