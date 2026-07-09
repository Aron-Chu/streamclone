# Hub live chat scale — ops runbook (2026-07)

Public contract for scaling live IRC coverage and hub Synced counts. Operator deploy lives in **private streampulse-ops**; this doc is safe for the public repo.

## Pre-deploy audit baseline (hosted)

Snapshot captured before code/env rollout:

| Signal | Value |
|--------|------:|
| `ingest.activeCollectors` | 250 |
| `ingest.tieringEnabled` | false |
| `poolSize` / live rows | 96 |
| Synced (hub UI) | ~40 |
| `roster.live` | ~106 |
| `roster.collectorTracking` | ~85 |
| `roster.liveCollectorDeficitRows` | ~21–24 |

**Interpretation:** IRC cap full; deficit + hub 96-row cap limit visible Synced count.

## Phase 1 — Env profile (tiering-on 500 scan / 250 IRC)

On hosted production VPS (private streampulse-ops checkout):

```bash
export STREAMPULSE_OPS_ROOT=/path/to/streampulse-ops
IMAGE_TAG=v0.3.0-rc27 bash /path/to/streamclone/scripts/ingest-phase-e-tiering-500-250-enable.sh
bash "${STREAMPULSE_OPS_ROOT}/scripts/smoke/hosted-limits-guard.sh"
```

Key env:

```env
INGEST_TIERING_ENABLED=1
HUB_ROSTER_LIMIT=500
INGEST_P1_HOT_LIMIT=250
INGEST_CANDIDATE_SCAN_TOP_N=500
MAX_ACTIVE_IRC_CHANNELS=250
PULSE_MAX_ACTIVE_CHANNELS=250
PUBLIC_HUB_LIVE_CAP=250
```

Do **not** use `ingest-phase-e-250-enable.sh` for this profile (sets tiering off; scheduler only scans MaxActiveIRC).

## Phase 2 — Slot composition audit

When `activeCollectors=250` but `liveCollectorDeficitRows>0`, audit slot holders:

| Bucket | How to inspect |
|--------|----------------|
| P0 protected / always-track | `analytics_always_tracked` count; ops admin UI |
| Manual watch (P1) | Collector refCounts / admin tracking list |
| Top roster P1 | Should dominate after tiering-on deploy |
| Extension watches | Low priority; evict stale via admin `POST .../tracking/{login}/evict` |
| Idle churn | Analytics logs: repeating PART/JOIN same login ~15m → need rc14+ |

**Remediation:** Trim excess P0/manual not in top live before raising IRC above 250.

## Phase 3 — Analytics image with code changes

Ship streamclone analytics image containing:

- `INGEST_CANDIDATE_SCAN_TOP_N` / scheduler scan decoupling
- `PUBLIC_HUB_LIVE_CAP=250` default
- IRC-first hub pool truncation

Promote via streampulse-ops `production-up.sh --no-deps analytics` with pinned `IMAGE_TAG`.

## Verification (public-safe)

```bash
curl -s https://api.streampulse.stream/v1/public/hub | jq '{
  tiering: .ingest.tieringEnabled,
  active: .ingest.activeCollectors,
  poolSize: .poolSize,
  liveRows: (.liveChannels | length),
  synced: [.liveChannels[] | select(.coverageState=="synced")] | length,
  roster: .corpusPipeline.roster
}'
```

| Signal | Target |
|--------|--------|
| `ingest.tieringEnabled` | `true` (until flat scan fix + tiering off desired) |
| `roster.liveCollectorDeficitRows` | `0` |
| `roster.collectorTracking` | ≈ `roster.live` |
| `poolSize` | up to **250** |
| Synced | bounded by live roster with chat rollups |

20m soak: `joinRate1m`/`partRate1m` stable, analytics RSS ~400–500 MiB at 250 IRC.

## Post-250 canary gates (do not skip)

Only after deficit = 0 and 20m soak PASS:

1. **500/500** — `PULSE_MAX_ACTIVE_CHANNELS=500`, `INGEST_P1_HOT_LIMIT=500`, re-benchmark RAM
2. **750** — next documented step ([`top-roster-idle-churn-p1-2026-07.md`](top-roster-idle-churn-p1-2026-07.md)); requires rc14+ idle fix

Soak gates each step: Redis `rejected_connections=0`, PG write p95, rollup flush p95, hub p95 clean, no 15m PART/JOIN waves.

## Rollback

- Restore ops production env backup from enable script
- `PUBLIC_HUB_LIVE_CAP=96` + prior image for emergency hub payload shrink
- Phase E 500/50 (RAM emergency): `scripts/ingest-phase-e-enable.sh`
