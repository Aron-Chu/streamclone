# StreamPulse Multi-User Infra Review

Status: canonical combined R0-R1 execution brief, updated 2026-06-23.

This plan is the BearHost-safe path for hosted StreamPulse. The goal is not to track
500 simultaneous live chats or run 500 VOD backfills. The goal is to maintain a
top/protected roster, join the subset that is live quickly, share one collector per
channel across many users, and use VOD backfill only as a repair path when Twitch
exposes replay chat.

## Plan Reconciliation

There were two useful planning artifacts, and they are not in conflict:

| Artifact | What it was | Verdict |
| -------- | ----------- | ------- |
| `streamclone-pulse/docs/Review.md` | A broad principal-engineer review prompt with product goals, pipelines, known gaps, and algorithm questions | Keep as the input context and challenge list |
| Initial infra review response | The full architecture answer: defaults, invariants, kill switches, storage, rollout, and BearHost capacity | Keep as the canonical architecture plan |
| Criticism pass | The sharper review of what was missing or under-prioritized | Adopt; it changes priority and clarifies risk |
| Plan Mode implementation block | A compressed execution plan for R0-a, R0-b, and R1 | Keep as the execution order, but restore the detail from the infra review |

Decision: this file is the merged source of truth for R0-R1. If an older note
conflicts with this file, use this file.

Key reconciliations from the criticism pass:

- Hosted quota enforcement is **Critical P0**, not medium or optional.
- Preemption is a requirement and test target, not something to assume is already
  built.
- `backfill_available` stays derived from `canBackfill && vodId` in R0; add a state
  only if client re-derivation bugs continue.
- BFF cache invalidation on VOD hint and backfill transitions is required and must be
  tested, even if part of it already exists.
- `PULSE_BFF_STALE_ON_ERROR` and Postgres write shedding are second wave; R0 needs
  read-only mode plus backfill/GQL kill switches first.
- Grafana is useful before raising beyond 25, but R0 can ship with health, structured
  logs, and rollback metrics.
- EventSub in emote code does not count as analytics go-live detection.
- The real no-VOD fix is fast live joining, measured by `tracked_from_start_ratio`
  and go-live-to-first-rollup latency.

## Execution Ledger

Build this as three goals, in order:

| Goal | Ship criteria | Do not widen until |
| ---- | ------------- | ------------------ |
| R0-a deploy freshness | Health reports version/build, hosted mode, `helixEnabled`, caps, and kill-switch states; hosted smoke catches `version=dev` and missing routes | Health is correct through the BearHost/Caddy route |
| R0-b hosted safety | Principal hashing, Redis quotas, VOD finalization, cache invalidation tests, and minimal kill switches are live | Hosted write APIs return honest `429 retryAfter`; no infinite `waiting_for_vod` |
| R1 protected go-live | Batched Helix roster poller joins on offline -> live and dedupes stream IDs | Cap 10 survives 24h smoke before raising to 25 |

Critical implementation priority:

```text
1. BearHost deploy freshness + health contract
2. Hosted quota enforcement
3. VOD finalization + BFF cache invalidation tests
4. Minimal kill switches
5. Helix protected go-live poller
6. Invariant tests and synthetic load gate
```

## Recommended Defaults

| Decision | Recommended default | Why | Revisit when |
| -------- | ------------------- | --- | ------------ |
| Primary hot store | Postgres rollups plus Redis cache/jobs | Already fits one 4 vCPU / 8 GB VPS and keeps source of truth simple | Postgres write p95 stays high after cap/backfill reductions |
| InfluxDB | Optional observability only, off for BearHost Pulse source of truth | Avoids making metrics storage part of the product data plane | Need high-cardinality dashboards after R1 is stable |
| ClickHouse/DuckDB/Parquet | Do not add for R0-R1 | The bottleneck is live join timing and quotas, not analytical storage | 30+ days of rollups become expensive to query/export |
| Kafka/Kubernetes | Do not add | Operational cost is higher than the current scale problem | One VPS cannot keep Q0 live path healthy at target cap |
| Helix vs EventSub | Helix polling first; EventSub later as optimization | Ships without webhook infrastructure and is enough for beta | Polling cost or latency becomes the cap blocker |
| MVP go-live poll interval | Batched Helix roster poll every 60-120s with backoff | Detects protected streams fast enough without per-channel polling | Need sub-30s go-live detection |
| Active live cap | Start 10, raise to 25 after 24h smoke | Matches BearHost profile and keeps memory/write pressure bounded | p95 latency and memory are stable at 25 for several days |
| Roster size | `PULSE_ROSTER_SIZE=500` | Supports top/protected discovery without joining 500 IRC channels | Product needs larger protected pool and metrics prove headroom |
| Global protected cap | `PULSE_PROTECTED_CHANNEL_LIMIT_GLOBAL=500` | Prevents unbounded always-track growth | Paid plan or bigger VPS arrives |
| Per-principal protected cap | `PULSE_MAX_ALWAYS_TRACKED_PER_PRINCIPAL=10` | Stops one beta key from consuming the roster | Abuse metrics are clean and quota model changes |
| Backfill concurrency | `PULSE_MAX_BACKFILLS=1` | GQL and Postgres load must not interfere with live tracking | Backfill success and write latency stay stable under load |
| GQL concurrency | 1 | Twitch replay comments are a fragile dependency | 429/error rate is low for a full beta cycle |
| VOD retry window | Retry 0s, 30s, 2m, 5m, then continue until 60m final window | Covers common Twitch archive delay without waiting forever | Real production delay distribution says otherwise |
| Final no-VOD policy | Mark `vod_unavailable` after 60m, plus honest live-side waiting/no-VOD copy | No VOD means historical recovery is not reliable | Twitch exposes better availability signals |
| `backfill_available` state | Do not add in R0; derive from `canBackfill && vodId` plus `copyKey` | Avoids another state unless clients keep re-deriving wrong copy | Portal/extension parity bugs persist after BFF coverage is canonical |
| Coverage cache | Keep 12s Redis TTL; invalidate on VOD hint and backfill completion | Good fanout story for many users watching one channel | Stale UX still confuses users after invalidation tests |
| `coverageVersion` | Add after P0 if stale coverage remains visible | Lets clients refresh immediately without dropping cache | Coverage card still lags during backfill transitions |
| Shared CoverageCard/copy | Extract to `pulse-core` after backend coverage contract settles | Prevents duplicated UX states across portal/extension | Both clients rely on identical stable coverage payloads |
| Provider abstraction before Kick | Do not build yet | Twitch live/VOD semantics are the current risk | Kick support becomes scheduled work |
| AI/LLMs | Never in ingest, VOD fetch, rollup, or scoring hot path | Deterministic live path must survive degraded services | Only offline analysis tooling needs it |

## Biggest Model

The main scaling model is:

```text
many users -> one BFF cache key -> one shared collector per live channel
```

The main product truth is:

```text
no VOD + not already live-tracked = not reliably recoverable
```

So the R1 success metric is not "can we backfill no-VOD streams." It is:

```text
tracked_from_start_ratio
go_live_detected_at -> first_rollup_written_at
```

## R0 Goals

1. BearHost deploy freshness and health contract.
   - Expected impact: eliminates stale deploy failures such as missing `helixEnabled`,
     `vod-hint`, or version fields.
   - Complexity: low.
   - Operational risk: low.
   - Proves worked: `/v1/extension/health` returns expected version, `helixEnabled`,
     hosted mode, build SHA, and enabled feature flags.
   - Regression guard: health contract test plus deploy smoke that fails on `version=dev`
     in production profile.

2. Hosted beta quota enforcement.
   - Expected impact: prevents one beta user from exhausting collectors or backfill.
   - Complexity: medium.
   - Operational risk: low if fail-closed for writes and fail-open only for reads.
   - Proves worked: 429 rate appears for deliberate quota tests; active channels and
     backfills remain under caps.
   - Regression guard: unit/integration tests for beta key auth, `principalId`, watch
     quota, backfill quota, and `retryAfter`.

   Required first slice:

   ```text
   principalId = sha256(betaKey)
   Redis token bucket for watch/protect
   Redis token bucket for backfill
   429 response with retryAfter
   principalId attached to watchlist/protect rows
   ```

3. Pulse-specific kill switches.
   - Expected impact: lets operators keep live Pulse stable when backfill, GQL, Helix,
     Redis, Postgres, or emote services degrade.
   - Complexity: low to medium.
   - Operational risk: low.
   - Proves worked: toggling each flag changes behavior without restart surprises.
   - Regression guard: config parsing tests and route/job behavior tests per flag.

4. VOD finalization and `vod_unavailable`.
   - Expected impact: stops infinite "waiting for archive" states.
   - Complexity: medium.
   - Operational risk: medium because copy and state transitions are user-visible.
   - Proves worked: VOD resolution reaches `available` or final `vod_unavailable`
     within policy.
   - Regression guard: fake clock tests for retry schedule and final transition.

5. BFF coverage correctness.
   - Expected impact: avoids client-side UX lies and stale "Load missed moments" states.
   - Complexity: low to medium.
   - Operational risk: low.
   - Proves worked: extension/portal render backend `coverage` payload without
     re-deriving primary state.
   - Regression guard: contract drift test and e2e fixture for partial, waiting,
     unavailable, running, and failed coverage.

## R1 Goals

1. Protected roster Helix poller.
   - Expected impact: improves no-VOD stream coverage prospectively by joining near
     go-live.
   - Complexity: medium.
   - Operational risk: medium due to Twitch rate limits and collector churn.
   - Proves worked: `tracked_from_start_ratio` increases and go-live-to-first-rollup
     p95 stays under target.
   - Regression guard: transition tests ensure only offline -> live creates a watch.

2. Cap raise from 10 to 25.
   - Expected impact: tracks more live protected/top-roster channels on the same VPS.
   - Complexity: low after metrics exist.
   - Operational risk: medium.
   - Proves worked: memory, Postgres write latency, Redis hit rate, and rollup flush
     latency stay within SLO for 24h.
   - Regression guard: synthetic 25-channel load test before cap raise.

3. Minimal observability before 25 -> 50.
   - Expected impact: makes capacity decisions evidence-based.
   - Complexity: medium.
   - Operational risk: low.
   - Proves worked: dashboard or log-derived report covers live, BFF, backfill, Helix,
     Redis, Postgres, and memory metrics.
   - Regression guard: smoke check fails if required metrics disappear.

## Scheduling Law

Q0 live tracking must never wait behind VOD backfill, archive/corpus jobs, context
enrichment, portal analytics, or emote enrichment.

Priority order:

```text
Q0: protected/live tracking, IRC joins, rollup flushes
Q1: user-triggered Load missed moments
Q2: VOD finalization/retry
Q3: context enrichment
Q4: archive/corpus/batch
```

Archive/corpus backfill is disabled on BearHost by default and must not share a queue
that can starve Q0.

## Go-Live Poller

Default implementation:

```text
Batch Helix Get Streams calls over roster channel IDs.
Cache broadcaster IDs; do not resolve logins every loop.
Persist last_live_stream_id and last_live_seen_at.
Create an internal watch only on offline -> live transition.
Ignore duplicate live observations for the same stream_id.
Back off using Twitch rate-limit headers and stop early on low remaining budget.
EventSub is later reconciliation/optimization, not R1 dependency.
```

Important note: EventSub may already exist in emote-related code. That does not mean
analytics go-live detection has EventSub coverage.

## VOD Resolution

Keep Redis for fast retry state and add sanitized Postgres history for debugging:

```text
vod_resolution_attempts:
  stream_id
  login
  twitch_stream_id
  candidate_vod_id
  source: helix | hint | extension_gql | page_dom
  status: resolving | available | unavailable | deleted | private | error
  attempts
  last_attempt_at
  final_after_at
  finalized_at
  error_code
```

Policy:

```text
No backfill without vodId.
broadcastId and streamId are never vodId.
Retry common archive delays for up to 60m.
Finalize to vod_unavailable after policy expires.
For live streams without a VOD yet, show honest waiting/no-VOD-risk copy.
```

## BFF And Coverage Contract

Backend `coverage` is canonical. Extension and portal must not re-derive primary
coverage state when the BFF sends coverage.

Default BFF SLOs:

```text
cache hit p95 < 75ms
cache miss p95 < 250ms
Redis hit rate >= 90% on popular channels
```

Keep the current 12s TTL, but test invalidation on:

```text
vodId hint accepted
backfill job completed
backfill job failed
coverage state finalized
```

Add `coverageVersion` only if stale UI remains visible after those invalidation tests.

## Kill-Switch Plan

| Flag | Default | Disables/degrades | User-visible mode |
| ---- | ------- | ----------------- | ----------------- |
| `PULSE_GQL_COMMENTS_ENABLED` | true | GQL VOD comment fetch | Load missed moments temporarily unavailable; live tracking still works |
| `PULSE_BACKFILL_ENABLED` | true | Pulse VOD backfill queue | Backfill temporarily unavailable |
| `PULSE_CONTEXT_ENRICHMENT_ENABLED` | false on BearHost | Context enrichment jobs | Labels/context may be incomplete |
| `PULSE_TOP_ROSTER_POLL_ENABLED` | false until R1 | Top-roster polling | Protected/manual watches still work |
| `PULSE_PROTECTED_GOLIVE_ENABLED` | false until R1 | Protected-channel go-live worker | Protect saved, but automatic join paused |
| `PULSE_EMOTE_ENSURE_BLOCKING` | false on BearHost | Blocking emote readiness for gold/backfill | Emote labels may lag; activity still tracked |
| `PULSE_BFF_CACHE_ENABLED` | true | Redis BFF caching | Higher DB load; lower cap if disabled |
| `PULSE_BFF_STALE_ON_ERROR` | later default true | Serve stale cache on backend error | Shows cached Pulse data with stale marker |
| `PULSE_READ_ONLY_MODE` | false | Mutating watch/protect/backfill APIs | Viewing works; new actions paused |
| `PULSE_WRITE_SHED_ENABLED` | later default false | Low-priority writes under PG pressure | Live rollups protected, optional writes paused |
| `PULSE_HELIX_ENABLED` | true | Helix VOD/live lookup | Live tracking can continue; VOD linking degraded |

For R0, implement the minimal set first: GQL comments, backfill queue, protected
go-live worker, emote blocking, read-only mode, and Helix.

## Invariant And Test Checklist

- [ ] `broadcastId` is never accepted as `vodId`; unit test VOD ID parser and API
  validation.
- [ ] `streamId` is never accepted as `vodId`; unit test parser and backfill request
  validation.
- [ ] No backfill job starts without a valid `vodId`; integration test backfill API.
- [ ] No fake progress; job progress is absent/unknown until real work reports it;
  UI fixture test.
- [ ] No raw chat text in extension/portal payloads; contract test response schemas.
- [ ] No client-side Pulse scoring; import/lint test or contract test around BFF score.
- [ ] No full timeline fetch during normal polling; integration test normal BFF path.
- [ ] One shared IRC session per channel; collector test duplicate watch requests.
- [ ] Protected channels preempt lower-priority channels; capacity governor test to add.
- [ ] Q0 live tracking cannot wait behind backfill/archive/context; scheduler priority
  test.
- [ ] Archive/corpus backfill cannot starve live Pulse tracking; queue isolation test.
- [ ] Hosted watch/protect quota is enforced per principal; Redis bucket test.
- [ ] Hosted backfill quota is enforced per principal; Redis bucket test.
- [ ] `principalId` is derived from beta key hash, not raw key; unit test and log scrub
  test.
- [ ] BFF cache invalidates on VOD hint; integration test API -> cache miss.
- [ ] BFF cache invalidates on backfill done/fail; integration test job transition.
- [ ] Extension and portal trust backend `coverage`; e2e fixture with backend state.
- [ ] Waiting VOD finalizes to `vod_unavailable`; fake clock test.
- [ ] Backfill dedupes by stream/range; backfill manager unit test.
- [ ] GQL 429/error backs off and does not retry storm; fake GQL client test.
- [ ] Helix poller batches roster checks; poller unit test asserts batched calls.
- [ ] Helix offline -> live transition creates one watch; poller integration test.
- [ ] Same stream ID observed twice does not duplicate join; poller integration test.
- [ ] Emote ensure cannot block backfill indefinitely; timeout metric/test.
- [ ] Postgres pressure disables low-priority work before Q0 live rollups; pressure
  circuit test.

## Storage And Retention

Store hot in Postgres:

- streams, stream coverage state, VOD resolution attempts, watch/protect rows,
  minute rollups, peak summaries, backfill job history, quota audit summaries.

Cache in Redis:

- BFF coverage/channel payloads, token buckets, active job locks, short-lived VOD retry
  state, active collector membership, stampede locks.

Export cold later:

- old minute rollups, historical job logs, derived aggregate reports.

Never store:

- raw chat text in extension/portal payloads, raw beta keys, OAuth tokens in logs,
  raw GQL blobs as product state, AI-generated scoring decisions in live path.

Cheap-first path:

```text
Postgres partitions or retention pruning first.
Redis TTL tuning second.
Cold CSV/Parquet export job third.
Only then evaluate DuckDB/ClickHouse for offline analytics.
```

## Synthetic Load Before 10 -> 25

Run before raising `PULSE_MAX_ACTIVE_CHANNELS`:

```text
25 fake/recorded channels
low / medium / high message-rate tiers
minute rollup flushes enabled
Redis BFF reads at 10x fanout
one backfill job running
Helix poller loop enabled with fake client
measure Postgres write p95, BFF hit/miss p95, memory, Redis hit rate
```

Raise the cap only if Q0 stays healthy.

## 24-Hour Watch Metrics

Track these during cap 10 smoke and cap 25 rollout:

```text
active_tracked_channels
protected_roster_size
tracked_from_start_ratio
go_live_to_first_rollup_seconds p50/p95
coverage_start_offset_seconds p50/p95
messages_per_second
rollup_flush_latency_p95
postgres_write_latency_p95
redis_bff_hit_rate
bff_hit_p95
bff_miss_p95
gql_429_rate
backfill_success_rate
backfill_queue_depth
irc_reconnects_per_hour
memory_percent
api_5xx_rate
api_429_rate
vod_unavailable_finalizations
```

Rollback triggers:

```text
memory > 85% for 10m
Postgres write p95 > 250ms for 10m
rollup flush p95 > 5s for 10m
BFF 5xx > 1% for 5m
IRC reconnect storm
GQL 429/error spike
Q0 live join latency exceeds target
```

Rollback action order:

```text
disable GQL comments
disable backfill queue
disable top-roster poller
reduce active cap to previous value
enable read-only mode for mutating hosted APIs
redeploy previous known-good analytics image if health contract regressed
```

## Cheap-First Architecture

Must do now on BearHost:

- deploy freshness and health contract
- hosted principal/quota enforcement
- minimal Pulse kill switches
- VOD finalization to `vod_unavailable`
- BFF coverage invalidation tests
- extension/portal coverage contract tests
- Q0 scheduler priority invariant

Can do later on BearHost:

- `coverageVersion`
- Grafana dashboards if Prometheus/Grafana tunnel is already available
- stale-on-error BFF mode
- write shedding for Postgres pressure
- shared CoverageCard/copy extraction to `pulse-core`
- cap 25 after smoke

Requires bigger VPS:

- stable cap 50+ with high message-rate channels
- multiple concurrent backfills
- longer hot retention without pruning

Requires new datastore/service:

- large historical analytical scans that Postgres cannot answer cheaply
- product need for long-retention high-cardinality event analytics
- cross-host collector coordination beyond one VPS

Do not do yet:

- Kafka
- Kubernetes
- ClickHouse as R0/R1 source of truth
- InfluxDB as product state
- provider abstraction for Kick
- AI/LLM scoring or ingest agents
- 500 simultaneous IRC channels on BearHost
- 500 concurrent VOD backfills

## References

- Twitch API concepts and rate-limit headers:
  https://dev.twitch.tv/docs/api/guide
- Twitch Videos API and archive availability:
  https://dev.twitch.tv/docs/api/videos
