# Launch-readiness task ledger — StreamPulse (2026-07-02)

Source: Fable 5 read-only architecture review (hosted probes + static inspection, 2026-07-02).
Audience: senior autonomous engineering agent working across `twitch-7tv-clone` (backend) and
`../streamclone-pulse` (portal/extension).

## How to work this ledger

- Execute in ID order within a priority band. Do not start a P1 task while an unblocked P0 remains.
- Every task lists **Acceptance** (must all pass) and **Verify** (exact commands). A task is not
  done until Verify passes against the hosted API or local tests as specified.
- **Soak guard:** a corpus 0B gold soak may be running on streampulse-vps. Tasks marked
  `SOAK-SAFE: yes` must not restart `streampulse-analytics-workers` or `streampulse-scraper`.
  Tasks that recreate only the `analytics` API container are soak-safe. Tasks marked
  `SOAK-SAFE: coordinate` need an operator window.
- **Operator boundary:** anything touching the VPS (`23.173.152.156`), BearHost
  (`141.11.243.103`), Cloudflare, or production env files is `OPERATOR`. Prepare the exact
  script/diff, verify locally, and stop; do not execute remotely without explicit approval.
- Repo rules apply: Conventional Commits, author `Aron-Chu <aroncloudchu@gmail.com>`, no
  Co-authored-by trailers, forward-only migrations, never commit `.env*` / secrets.
- Run `make check-quick` in the backend repo and `npm run typecheck && npm test` in
  `streampulse-web` before declaring any band complete.
- Keep the **traceability checklist** at the bottom current when adding or completing tasks;
  it is the quick audit map from launch-readiness finding to implementation task.

---

## P0 — launch blockers

### LB-01 — Admission env vars never reach the analytics container

- **Severity:** Critical · **Confidence:** confirmed · **SOAK-SAFE:** yes (analytics API only)
- **Problem:** `PULSE_TOP500_ADMISSION_ENABLED`, `PULSE_TOP500_ADMISSION_TOP_N`,
  `TOP500_METADATA_TOP_N` / `CORPUS_TARGET_TOP_N` are not referenced in any compose
  `environment:` block for the `analytics` service, and its only `env_file` is `.env`.
  Values written to `deploy/env/profile-streampulse-vps-production.local.env` affect compose
  *interpolation only*, so admission stayed off (`admissionDisabled == live` in the hub;
  `StartTop500PriorityWatchPoller` gated at `cmd/analytics/main.go:574`). Meanwhile
  `PULSE_MAX_ACTIVE_CHANNELS` *does* land via `deploy/docker-compose.bearhost-pulse.yml:17`.
- **Fix:** in `deploy/docker-compose.streampulse-vps-production.yml` add to the `analytics`
  service `environment:` block (interpolated so the local env file controls them):
  ```yaml
  PULSE_TOP500_ADMISSION_ENABLED: ${PULSE_TOP500_ADMISSION_ENABLED:-false}
  PULSE_TOP500_ADMISSION_TOP_N: ${PULSE_TOP500_ADMISSION_TOP_N:-100}
  PULSE_TOP500_ADMISSION_INTERVAL: ${PULSE_TOP500_ADMISSION_INTERVAL:-60s}
  TOP500_METADATA_TOP_N: ${TOP500_METADATA_TOP_N:-100}
  CORPUS_TARGET_TOP_N: ${CORPUS_TARGET_TOP_N:-0}
  ```
  Keep the workers service pinned off (it already hardcodes collector caps to 0). Update
  `deploy/env/profile-streampulse-vps-production.env.example` comments to say these keys are
  now interpolated. Note in docs: `PULSE_COLLECTOR_ENABLED` is a **no-op** in Go — remove it
  from examples or wire it; do not leave a dead knob documented as a control.
- **Acceptance:** `docker compose ... config` renders the vars on `analytics`;
  `make compose-config-check` passes; docs/examples updated.
- **Verify (local):** `make compose-config-check`; grep rendered config for
  `PULSE_TOP500_ADMISSION_ENABLED`.
- **Verify (OPERATOR, after deploy + setting values in local env):**
  ```bash
  docker exec <analytics-container> printenv | grep -E 'PULSE_TOP500|TOP500_METADATA|CORPUS_TARGET'
  docker logs <analytics-container> 2>&1 | grep 'top500 priority watch poller enabled'
  curl -s 'https://api.streampulse.stream/v1/public/hub?activityWindow=30m' \
    | jq '.corpusPipeline | {topN, collectorActive, collectorMax, roster}'
  # expect: admissionDisabled=0, collectorTracking>0 within ~10 min while channels are live
  ```

### LB-02 — Two processes write the public hub Redis cache (hub truth flaps)

- **Severity:** Critical · **Confidence:** confirmed (architecturally; observed collectorMax 50↔200 flip)
- **SOAK-SAFE:** coordinate (workers container recreate; schedule between gold jobs)
- **Problem:** `handler.StartPublicCacheRefresh(ctx)` runs unconditionally
  (`cmd/analytics/main.go:592`). Both the API container and `streampulse-analytics-workers`
  refresh `sp:public:hub:activity:{30,10080}` every 60s into shared Redis. The workers process
  has collector cap 0 and admission unset, so its snapshots report `collectorTracking=0` /
  wrong `collectorMax` even when the API container is tracking. Roster/collector fields come
  from the *writer's* in-memory collector (`top500_readiness.go:86-89`).
- **Fix:** gate the refresh loop off in worker processes. Minimal:
  ```go
  // cmd/analytics/main.go — replace the unconditional call
  if !corpusWorkersEnabledForThisProcess() { // CORPUS_WORKERS_ENABLED != "1"
      handler.StartPublicCacheRefresh(ctx)
  } else {
      logger.Info("public cache refresh disabled: corpus worker process")
  }
  ```
  (Reuse/extend `corpusWorkersExplicitlyDisabled()` at `main.go:618`; the API sets
  `CORPUS_WORKERS_ENABLED=0`, workers set `1`.) Add a unit test asserting the gate. On-demand
  HTTP cache fills in the worker are unreachable via Caddy (routes pin `analytics:8080`), so
  the refresh loop is the only cross-writer.
- **Acceptance:** `go test ./cmd/analytics/... ./internal/analytics/...` green; grep confirms
  no other unconditional `StartPublicCacheRefresh` callers.
- **Verify (OPERATOR):** after deploy, poll
  `curl -s '.../v1/public/hub?activityWindow=30m' | jq '.corpusPipeline.collectorMax'` every
  60–90s for 10 min → value must be stable and match the API container's cap.

### LB-03 — `/v1/public/hub/moments` returns "empty" for buckets the chart shows active

- **Severity:** Critical (product honesty) · **Confidence:** confirmed symptom, root cause needs SQL
- **SOAK-SAFE:** yes
- **Problem:** live probe: bucket `1782960480000` had `chat=5873` in `/v1/public/hub`
  (`activityWindow=7d`), but `/v1/public/hub/moments?bucketT=1782960480000&activityWindow=7d`
  returned `status:"empty", reason:"no_corpus_peaks_in_bucket"`. Both paths read
  `analytics_minute_rollups` with `sqlPublicLiveChatMinutePredicate`. Two code defects amplify
  whatever the root cause is:
  1. `buildPublicHubMoments` (`internal/analytics/hub_historical_moments.go`) swallows store
     errors — a failing query is reported as an empty bucket.
  2. Empty responses are negative-cached in Redis for the full 2-minute TTL
     (`loadPublicHubMoments`), including while backfill is still filling the window.
- **Fix (code, in order):**
  1. Propagate store errors: on query error return HTTP 503 `hub_moments_unavailable` and log
     `err` with `bucketT`, window, and the computed `[start,end)`. Never map error → `empty`.
  2. Cache empties with a short TTL (20–30s) or skip caching when
     `reason == "no_corpus_peaks_in_bucket"`; keep 2 min for non-empty payloads.
  3. Add a bucket-grid consistency test: for a fixed `windowMinutes`, assert
     `hubBucketTimeRange(t)` reproduces exactly the bucket keys emitted by
     `AggregateRollupBucketsSince`'s SQL grouping (epoch-floor on the same bucket width and
     the same window anchor). If the SQL anchors buckets to `since` rather than epoch 0 (or
     vice versa), that off-by-anchor is the likely root cause — fix `hubBucketTimeRange` to
     match the SQL, add regression test with synthetic rollups.
- **Diagnosis (OPERATOR, read-only SQL on VPS Postgres) — run before/alongside the fix:**
  ```sql
  SELECT count(*), min(r.minute_ts), max(r.minute_ts)
  FROM analytics_minute_rollups r
  JOIN analytics_streams s ON s.stream_id = r.stream_id
  WHERE r.minute_ts >= to_timestamp(1782960480) AND r.minute_ts < to_timestamp(1782963000)
    AND r.chat_count > 0;
  -- then re-run with the exact sqlPublicLiveChatMinutePredicate appended.
  -- rows>0 without predicate but 0 with predicate → chat_source stamping issue on live rollups.
  -- rows=0 either way → bucket window math drift (fix #3 above).
  ```
- **Acceptance:** new Go tests pass (`error → 503`, `empty TTL short`, `grid consistency`);
  `make test-analytics` green.
- **Verify (OPERATOR):** pick a bucket with `chat>0` from the live hub payload; moments
  endpoint returns either ≥1 moment or a *diagnosable* empty (logs show real row counts).

### LB-04 — `momentsDetected` KPI counts bookmarks (shows 0 on hosted)

- **Severity:** High (public honesty) · **Confidence:** confirmed · **SOAK-SAFE:** yes
- **Problem:** `PublicAggregateStats` (`internal/analytics/public_api.go:240-246`) fills
  `MomentsDetected` from `COUNT(*) FROM pulse_bookmarks` → hosted hub shows
  `momentsDetected: 0` next to 23M messages.
- **Fix:** two-step.
  1. **Now:** stop displaying it. In `streampulse-web`, remove/relabel any KPI bound to
     `corpus.momentsDetected` (search `momentsDetected` under `streampulse-web/src`). If a
     count is wanted, label it "Bookmarks".
  2. **Backend (after T2-04 partial index exists):** replace the subquery with a real,
     cheap peak count, e.g. `SELECT COUNT(*) FROM analytics_minute_rollups WHERE chat_count >=
    <peak threshold>` bounded to a window, or maintain a counter table updated by rollup
     ingest. Keep the JSON field name only if its semantics become true.
- **Acceptance:** portal no longer renders a zero "moments detected" KPI; backend change has a
  test pinning the new SQL semantics.
- **Verify:** `curl -s .../v1/public/hub | jq .corpus` + portal screenshot of the KPI row.

### LB-05 — Production deploy preflight blocks on its own containers

- **Severity:** High (ops) · **Confidence:** confirmed · **SOAK-SAFE:** yes (script-only)
- **Problem:** `streampulse_vps_corpus_worker_conflicts`
  (`scripts/lib/streampulse-vps-production-compose.sh:101-121`) flags containers named
  `streampulse-analytics-workers` / `streampulse-scraper` — names the production overlay
  itself assigns (`deploy/docker-compose.streampulse-vps-production.yml:48,89`). Every full
  redeploy after first bring-up self-conflicts, which forced the rsync + targeted-rebuild
  workaround.
- **Fix:** match on the compose *project label*, not container name:
  ```bash
  docker ps -a --filter 'label=com.docker.compose.project=streamclone-corpus' \
    --format '{{.Names}}|{{.Status}}'
  ```
  Treat only the legacy `streamclone-corpus` project (and the legacy
  `streamclone-pulse-irc-collector` container) as conflicts. Add
  `streamclone-collector`/legacy-collector detection while here (currently missed).
- **Acceptance:** shellcheck clean; a dry-run against a fake `docker ps` fixture (unit-style
  bash test or manual transcript in the PR description) shows production containers are not
  flagged and legacy ones are.
- **Verify (OPERATOR):** `bash scripts/streampulse-vps-production-deploy.sh` preflight passes
  with the production stack running.

### LB-06 — Portal on Cloudflare Pages is stale (no historical-moments UX)

- **Severity:** High · **Confidence:** confirmed (deployed bundle `index-BX_JrzPq.js` lacks
  `hub/moments` strings) · **SOAK-SAFE:** yes
- **Depends on:** LB-03 (do not ship the bucket-click UX while the backend returns false
  empties, unless the panel's "peaks unavailable" copy is verified).
- **Fix:**
  1. Strengthen `streampulse-web/scripts/pages-deploy-prod.mjs` to run the same gate as CI
     before `vite build`: `tsc --noEmit`, `check:analytics-routes-spa`,
     `check:analytics-links`, `check-backend-url` (it currently runs only build +
     backend-url).
  2. `OPERATOR`: `cd streamclone-pulse/streampulse-web && npm run pages:deploy:prod` with
     `CLOUDFLARE_API_TOKEN` set.
- **Acceptance:** deploy script refuses to ship on typecheck/link-check failure (test by
  temporarily breaking a type locally — do not commit the breakage).
- **Verify (post-deploy):** new bundle hash on `https://streampulse.stream/`; bundle contains
  `hub/moments`; click a historical chart bucket → moments panel loads or shows the honest
  banner; emote tiles render with fallback on 404.

---

## P1 — before top-200

### T2-01 — Top-500 metadata sampler staleness (all live rows stale)

- **Confidence:** likely · **SOAK-SAFE:** yes
- Hub showed `metadataStale=18/18` (staleness = `now > stale_after`, sampled_at + 15m).
  Diagnose why the sampler isn't ticking: check `TOP500_METADATA_ENABLED` wiring the same way
  as LB-01 (it may be another interpolation-only knob), Helix creds in the analytics
  container, and sampler logs. Fix the plumbing; add a hub field or metric
  `metadataSampledAgoSeconds` so staleness is visible without SQL.
- **Verify:** SQL from the admission review (`top500_current` stale counts) drops to ~0;
  `coverage.state` leaves `critical` once admission (LB-01) is also on.

### T2-02 — Make admission explain itself in the hub payload

- **SOAK-SAFE:** yes
- Add to `HubCorpusPipeline` (`internal/analytics/hub_overview.go`, populate in
  `buildHubCorpusPipeline` from `corpusRuntimeConfig()`): `liveAdmissionEnabled`,
  `liveAdmissionTopN`, `maxActiveIrcChannels`. Rename the misleading bulk roster counter:
  emit `admissionFeatureDisabled: true` when `!LiveAdmissionEnabled` instead of setting
  `admissionDisabled = liveRows` (`top500_readiness.go:108-110`); keep the old JSON key
  populated for one release with a deprecation note if the portal reads it (check
  `streampulse-web` usages first). Update portal copy for `critical` state: "Live tracking is
  currently offline — historical corpus data is still available."
- **Verify:** hub JSON shows the three new fields; `TestPublicHubResponseOmitsSensitiveKeys`
  still passes (no secrets leaked).

### T2-03 — Missing admission observability counters

- **SOAK-SAFE:** yes
- Extend `metrics/top500_admission.go` + poller to increment counters for: `disabled`
  (poller gated off — emit once per tick from a gauge/log), `rate_limited`,
  `collector_unhealthy` (IRC manager down), `env_mismatch` (admission on but collector cap
  0), `no_oauth` (Helix/auth unavailable when admission depends on live metadata), and
  `lease_conflict` only if/when collector leases are wired. `warming` stays a readiness
  state, but must be exposed in readiness JSON as a count. Log one structured line per
  admission cycle: considered / admitted / skipped-by-reason.
- **Verify:** `curl -s localhost:8087/metrics | grep top_roster_admission` in local stack.

### T2-04 — Postgres time-window indexes for rollups

- **SOAK-SAFE:** coordinate (index build load) · **Migration:** forward-only, new file
- New migration (must run **non-transactionally** for `CONCURRENTLY` — split into its own
  migration with the golang-migrate `-- +migrate` no-transaction convention used in this
  repo, or document operator-applied DDL if the migration runner wraps in a transaction):
  ```sql
  CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_analytics_rollups_minute_ts
    ON analytics_minute_rollups (minute_ts);
  CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_analytics_rollups_window_hot
    ON analytics_minute_rollups (minute_ts, chat_count DESC)
    WHERE chat_count > 0;
  ```
  These serve `AggregateRollupBucketsSince` (hub chart) and
  `TopHistoricalChatMinutesInWindow` (bucket moments), which currently scan.
- **Verify (OPERATOR):** `EXPLAIN` both queries before/after; `pg_indexes` lists both; hub 7d
  rebuild p95 drops (watch analytics logs / `pg_stat_statements` if enabled).

### T2-05 — Gold backlog drain plan

- **SOAK-SAFE:** coordinate · **Blocked by:** current 0B soak completion
- After soak sign-off: set `BACKFILL_GOLD_WORKER_COUNT=2` (interpolated already, workers
  overlay line 54), watch GQL 429 rate and `oldestQueuedSeconds` for 48h. Triage the 2,806
  failed silver jobs: classify retriable vs permanent
  (`SELECT last_error, count(*) FROM backfill_jobs WHERE tier='silver' AND status='failed'
  GROUP BY 1 ORDER BY 2 DESC LIMIT 20;`), bulk-requeue retriable classes via existing
  maintenance path, mark permanent ones skipped with a reason.
- **Exit criteria for top-200 IRC widening:** gold `oldestQueuedSeconds < 172800` (2 days)
  and queue depth trending down for 72h.

### T2-06 — Decorate VOD-pulse and heatmap emote URLs

- **SOAK-SAFE:** yes
- `buildExtensionVodPulse` (`internal/analytics/extension_vod_pulse.go:184-219`) and
  `heatmap/emotes.go:36-37` emit raw relative `/emotes/…` URLs. Route both through
  `decorateExtensionEmotesBatch` / `rewriteHostedTopEmotes` like the hub path. Add a test
  asserting no payload `imageUrl` starts with `/` when `CDN_PUBLIC_BASE` is set.
- **Verify:** `curl -s .../v1/extension/pulse/vod/<vodId> | jq '..|.imageUrl? // empty' |
  grep '^"/' ` returns nothing.

### T2-07 — Portal emote `onError` coverage + localStorage staleness

- **SOAK-SAFE:** yes · Repo: `streamclone-pulse`
- Add the existing fallback pattern (`PeakEmoteImage`) to: `TopEmotesPanel.tsx`,
  `TopEmoteBurstsPanel.tsx`, `MomentsFeed.tsx`, `HubRailCards.tsx`, `EmoteEconomyCard.tsx`,
  `FigmaChannelDashboard.tsx`. Extract a shared `EmoteImg` component instead of a seventh
  copy. Also: either surface the computed-but-unused `stale` flag from `publicHubCache.ts`
  (10 min) in the hub banner, or discard cache entries older than the stale threshold.
- **Verify:** `npm test`; manual: block `cdn.7tv.app` in devtools → text fallbacks, no broken
  image icons.

### T2-08 — Hub cache pre-warm for 24h window + moments TTL alignment

- **SOAK-SAFE:** yes
- `refreshPublicCaches` (`public_api.go:71-79`) pre-warms only 30m and 7d. Add 24h (1440).
  Include `hubGeneratedAt` in the `/hub/moments` response so the portal can detect skew
  against the chart snapshot it rendered from.

### T2-09 — Public source contract and coverage-state labels

- **SOAK-SAFE:** yes · Repos: backend + `streamclone-pulse`
- The review found an unresolved contract conflict: the chart and historical moments currently
  use a live-only predicate, while the public product copy also claims corpus-backed history.
  Make this explicit before top-200:
  1. Define the public contract in docs (`docs/website-portal/` in `streamclone-pulse` and a
     backend note near `docs/agent-notes/launch-readiness-2026-07-02.md`): which surfaces are
     `live_irc`, which are `gql_gold`/corpus-backed, and which are labeled hybrid.
  2. Add `source` / `sourceLabel` expectations to `/v1/public/hub`, `/v1/public/hub/moments`,
     channel session analytics, and extension live panel tests. If bucket moments include gold
     minutes, the activity chart must include the same source set for the same bucket; do not
     silently mix source sets.
  3. Standardize one-sentence UI labels:
     - operational: "Live chat tracking is active across the top-N roster."
     - degraded: "Live tracking is running with reduced coverage; some channels are not being followed right now."
     - critical: "Live tracking is currently offline — historical corpus data is still available."
  4. Remove or qualify unsafe claims until backed by data: "top 200", "top 500", "live",
     and "moments detected". Keep safe claims only with scope: "23M messages analyzed",
     "historical", "corpus-backed" for completed VODs, and "most reacted among tracked channels".
- **Acceptance:** docs and tests pin the source set per public surface; portal copy never calls
  a corpus/gold-only surface "live"; the hub cannot show chart data and bucket moments from
  different source contracts without labeling the mismatch.
- **Verify:** `go test ./internal/analytics/...`; `cd ../streamclone-pulse/streampulse-web &&
  npm run typecheck && npm test`.

### T2-10 — Edge block regression smoke for hosted routes

- **SOAK-SAFE:** yes
- The review confirmed raw-chat/ops routes are blocked now, but this should become a regression
  gate instead of a one-off probe. Add or extend hosted smoke scripts so CI/operator smoke checks
  assert:
  - `/v1/analytics/streams/*/chat-replay` and `/chat-messages` return 404.
  - `/metrics` returns 404.
  - `/v1/internal/corpus/gaps` returns 401/403 without token, not 200.
  - `/v1/admin/pulse/*` requires app-layer auth and does not leak operator fields.
  - Unknown paths under the API host return 404 after T3-06.
- **Acceptance:** smoke script exits non-zero on any exposure; docs mention it in the hosted
  post-deploy checklist.
- **Verify:** `PULSE_SMOKE_BASE_URL=https://api.streampulse.stream bash
  scripts/pulse-hosted-boundary-smoke.sh` (or the new script name).

---

## P2 — before top-500

### T3-01 — Enforce GQL rate caps that are currently config-only

- `GOLD_GLOBAL_GQL_RPM`, `GOLD_PER_VOD_GQL_RPM`, `GOLD_MAX_PARALLEL_VODS` are parsed/validated
  (`internal/config/config.go:228-231,455-471`) but never referenced at runtime. Wire a global
  token-bucket into `gqlRateCoordinator` (`sync_gql_parallel.go`) honoring `GOLD_GLOBAL_GQL_RPM`
  across workers (Redis-backed if multi-host BearHost drain is active). Until this lands, do
  not raise `BACKFILL_GOLD_WORKER_COUNT` above 2.

### T3-02 — Wire `BACKFILL_SILVER_WORKER_COUNT`

- Documented in compose but unread in Go (`cmd/analytics/main.go:380-382` starts exactly one
  silver worker). Implement the loop like gold's, clamp ≤4, default 1.

### T3-03 — Materialized read model for hub chart + peaks table for moments

- Hourly pre-aggregated activity buckets (refresh loop or trigger-fed) to take 7d/30d chart
  rebuilds off raw rollups; `analytics_minute_peaks` (top-K per bucket, populated at rollup
  ingest) so bucket clicks never sort raw windows or drag `emotes_json`. Cut
  `TopHistoricalChatMinutesInWindow` over to the peaks table behind a flag.

### T3-04 — Emote CDN domain cutover (EMOTE-R2-005)

- Decide final URL shape first (`/emotes/{uuid}/{scale}.webp` as implemented vs
  `/{provider}/{id}/{scale}.webp` as documented) — 24h-immutable caching makes later shape
  changes painful. Then: mint `cdn.streampulse.stream` → set
  `CDN_PUBLIC_BASE=https://cdn.streampulse.stream/emotes` → after verification, delete the
  portal's `absolutizeEmoteAssetUrl` API-host guessing (API must then always return absolute
  or omit `imageUrl`). Update `docs/storage/emotes-r2-migration.md` as steps land.

### T3-05 — Backfill job lease tightening

- `backfill_jobs` has no lease columns; stale `running` reclaim is 2h on `updated_at`. Before
  top-500 job volume: shrink `BACKFILL_STALE_RUNNING_AFTER` to ~15m and/or add
  `lease_owner`/`lease_expires_at` (new migration) mirroring the segment-store pattern.

### T3-06 — Caddy: 404 for unmatched paths

- `deploy/Caddyfile.pulse-api` falls through to Caddy's default 200-empty for unknown paths
  (observed on `/grafana/*`). Add a terminal `respond 404` after all matchers. Cosmetic but
  removes false "endpoint exists" signals from scanners and probes.

### T3-07 — Availability alerts for the truth plane

- Add low-noise alerting/runbook checks for the signals that made this audit hard:
  - admission enabled gauge is 0 while live roster rows > 0;
  - hub cache writer identity is not `analytics-api` (requires LB-02 to stamp writer role);
  - `metadataStale/live > 0.25` for more than 10 minutes;
  - gold `oldestQueuedSeconds > 172800`;
  - `DEPLOYED_SHA` older than `origin/master` by more than one approved deploy window;
  - cloudflared systemd health failing while compose services are healthy.
- Prefer hosted smoke + Prometheus/cron summaries over noisy paging until launch traffic
  justifies alerts. Document exact operator commands and expected thresholds.
- **Verify:** a read-only `scripts/tmp/hosted-launch-probes.sh` (or promoted equivalent)
  prints all six truth-plane checks and exits non-zero on failure.

---

## P3 — hardening

- **H-01** Scheduled `pg_dump` from streampulse-vps to BearHost + documented tunnel-flip
  drill; without this the rollback host serves silently stale data.
- **H-02** CI-gated Pages deploy: GitHub Action running the full check suite +
  `pages:deploy:prod` on tag/dispatch, followed by hosted smoke
  (`deploy/smoke/bearhost-pulse-api.sh` with `PULSE_SMOKE_BASE_URL`, `hosted-launch-probes`).
- **H-03** DEPLOYED_SHA drift alert: cron or smoke step comparing `DEPLOYED_SHA` on the VPS
  to `origin/master`; warn after N days of drift.
- **H-04** Promote tunnel-token rotation out of `scripts/tmp/` (rsync excludes `scripts/tmp/`,
  so emergency scripts are absent on the host); rotate the BearHost-reused tunnel token.
- **H-05** Wire the existing `CollectorLeaseManager` if a second IRC collector host is ever
  planned; today the in-process collector is a documented SPOF.
- **H-06** Add hosted-data read-only MCP entry to the streamclone
  `.cursor/mcp.recommended.json.example` (exists only in the pulse repo example).

---

## Traceability checklist

Use this to verify the ledger still covers the launch-readiness review.

| Review issue | Task(s) |
|---|---|
| Admission env vars do not reach `analytics`; `admissionDisabled == live` | LB-01 |
| Hub public cache has two writers with different in-memory collector state | LB-02 |
| Chart bucket has activity but `/hub/moments` returns false empty | LB-03 |
| Historical moments negative-cache empty buckets and hide store errors | LB-03, T2-08 |
| Portal production bundle is stale / no historical-moments UX | LB-06 |
| `momentsDetected` counts bookmarks, not detected peaks | LB-04 |
| `corpusPipeline.topN` can diverge from admission top-N | LB-01, T2-02, T2-09 |
| Public source contracts and product claims need live/corpus labels | T2-09 |
| Metadata sampler stale rows drive critical state | T2-01, T3-07 |
| Missing admission counters (`disabled`, `no_oauth`, `lease_conflict`, `collector_unhealthy`, `rate_limited`, `env_mismatch`) | T2-03 |
| Gold backlog is old; drain should precede top-200 IRC | T2-05 |
| `GOLD_GLOBAL_GQL_RPM` / per-VOD caps are config-only | T3-01 |
| `BACKFILL_SILVER_WORKER_COUNT` is documented but not wired | T3-02 |
| `analytics_minute_rollups` time-window scans lack indexes | T2-04 |
| Need read model / peaks table before top-500 | T3-03 |
| Portal localStorage stale flag unused | T2-07 |
| Hub cache pre-warms only 30m and 7d | T2-08 |
| Raw chat / ops edge blocks need regression guard | T2-10 |
| Full deploy preflight self-conflicts with production container names | LB-05 |
| BearHost rollback would be stale without fresh dump/tunnel drill | H-01 |
| `DEPLOYED_SHA` drift and hosted smoke are not alerting | H-02, H-03, T3-07 |
| Tunnel token reuse / rotation script still in `scripts/tmp` | H-04 |
| Emote hub URLs work, but VOD pulse and heatmap can emit relative paths | T2-06 |
| Final R2/CDN URL contract and custom domain cutover incomplete | T3-04 |
| Several portal emote images lack `onError` fallback | T2-07 |
| `backfill_jobs` lease/heartbeat is coarse | T3-05 |
| `/grafana/*` / unknown paths return 200-empty due to Caddy fallthrough | T3-06 |

---

## Sequencing summary

```
LB-01 ──┬── LB-02 ── (deploy analytics + workers window) ── verify hub truth
        │
LB-03 ──┼── LB-06 (portal deploy last, after backend honest)
LB-04 ──┤
LB-05 ──┘   (script-only, anytime)

P1: T2-01..03 (truth plane) → T2-04 (indexes) → T2-05 (gold drain) → T2-06..10
P2 gates top-500: T3-01 + T3-02 + T3-03 before CORPUS_TARGET_TOP_N=500
```

Definition of "launchable": LB-01…LB-06 verified on hosted, hub `coverage.state` operational
with truthful collector numbers, bucket clicks never contradict the chart, and the portal
serves the current bundle.
