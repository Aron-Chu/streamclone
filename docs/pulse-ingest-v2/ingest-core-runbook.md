# Phase C shadow validation (no cutover)

Staged rollout for bounded live ingest (`internal/analytics/ingestcore/`).

## Phases

| Phase | Env | Goal |
|-------|-----|------|
| A/B | `INGEST_CORE_ENABLED=0` | Package lands; zero behavior change |
| C | `INGEST_CORE_DUAL_READ_MODE=1`, `INGEST_CORE_SHADOW_MODE=1`, `INGEST_CORE_ENABLED=0` | Legacy writes PG; ingest-core shadow artifacts |
| D | `INGEST_CORE_ENABLED=1`, dual/shadow off, `INGEST_TIERING_ENABLED=0` | 250/250 cutover |
| E | `INGEST_TIERING_ENABLED=1`, `HUB_ROSTER_LIMIT=500`, `MAX_ACTIVE_IRC_CHANNELS=50` | 500 roster / 50 IRC |

**Hard stops:** Do not enable `INGEST_CORE_ENABLED=1` until Phase C gates pass. Do not flip 500/50 until Phase D soak passes. Do not combine code deploy with scale-policy changes.

## Step 0 — read-only baseline

On VPS (streampulse-ops):

```bash
bash scripts/ingest-phase-c-step0-baseline.sh
```

Saves docker/redis/df/postgres/public-api/Prometheus snapshots under `runtime/evidence/ingest-core-phase-c-step0-*`.

Missing `ingest_*` Prometheus metrics **before** the shadow image is **metric absent pre-release** — not a failure.

## Step 1 — preconditions (before shadow env)

```bash
bash scripts/ingest-phase-c-preconditions.sh
```

| # | Task | Notes |
|---|------|-------|
| 1 | Moments `Cache-Control` on prod | Required before shadow |
| 2 | Docker limits | `deploy/compose/hosted-resource-limits.compose.yml` — **one service at a time**, wait **15–30 min**, recapture stats/logs/hub/redis delta |
| 3 | Corpus/scraper off API node | `CORPUS_WORKERS_ENABLED=0`, `SCRAPER_ENABLED_ON_API_NODE=0` |
| 4 | `INGEST_CORE_ENABLED=0` | Default until Phase D |

**Do not** apply Docker limits and shadow env in the same restart.

Env template: `deploy/env/profile-hosted-ingest-core-phase-c.env.example`

## Step 2 — Phase C shadow deploy

1. Release tag with ingest-core + moments Cache-Control + allowlist fix.
2. Local gate: `go test ./internal/analytics/ingestcore/... ./internal/analytics/...` and `go build ./cmd/analytics`.
3. Deploy image via **streampulse-ops** (pinned `IMAGE_TAG`).
4. Merge Phase C env overlay; restart analytics **only** (not combined with limits change).
5. Staged allowlist:

| Window | Allowlist |
|--------|-----------|
| ~1h | 5–10 channels |
| 3–6h | top 50 |
| 12h | full 250 (empty allowlist) |

**Phase C does not write PG** — verify:

- `INGEST_CORE_ENABLED=0`
- `INGEST_CORE_DUAL_READ_MODE=1`
- `INGEST_CORE_SHADOW_MODE=1`
- Startup log: `ingest-core active` with `core_enabled=false`
- Legacy collector remains sole PG writer

Shadow artifacts: `runtime/ingest-shadow/latest.jsonl` (rotates at 128MiB).

## Step 3 — Phase C gates

```bash
bash scripts/ingest-phase-c-gates.sh 2 1000
bash scripts/ingest-shadow-compare.sh 2 1000
du -sh runtime/ingest-shadow/
```

| Gate | Pass |
|------|------|
| Shadow compare | ≥99% match |
| Drops | `rate(ingest_messages_dropped_total[5m])` ~0 |
| Redis | `rejected_connections` delta flat vs Step 0 |
| Memory | analytics RSS flat |
| Rollup p95 | not worse than Step 0 baseline |
| Hub/moments | healthy; Cache-Control present |
| Artifacts | bounded; rotation working |
| Logs | no repeated parser/flush/queue errors |

Output includes **PHASE_D_GO_NOGO: GO/NO-GO**.

## Phase D cutover (later)

Only after Phase C gates pass. Flip `INGEST_CORE_ENABLED=1`, dual/shadow off, 250/250 unchanged. Soak 12–24h.

## Rollback

```env
INGEST_CORE_ENABLED=0
INGEST_CORE_DUAL_READ_MODE=0
INGEST_CORE_SHADOW_MODE=0
```

Recreate analytics with prior env or prior `IMAGE_TAG`. No DB migration required.

## Metrics

- `ingest_active_collectors`, `ingest_desired_collectors`, `ingest_admit_lag_seconds`
- `ingest_irc_queue_age_seconds`, `ingest_shard_queue_age_seconds`, `ingest_flush_queue_age_seconds`
- `ingest_messages_dropped_total{tier}`
- `analytics_rollup_write_batch_size{kind="ingest_batch"}`

## Env reference

- `deploy/env/profile-hosted-ingest-core.env.example` — Phase D/E
- `deploy/env/profile-hosted-ingest-core-phase-c.env.example` — Phase C shadow
