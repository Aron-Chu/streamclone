# Pulse 24h soak runbook (LOAD-001b → CAP-001)

Operator-only. Run after **LOAD-001b staging-25 PASS** on the isolated `:8091` stack (or document if soaking production cap-10 baseline instead).

## 1. Start

```bash
# On legacy-rollback-host
bash scripts/bearhost-pulse-staging-up.sh   # cap 25, localhost only
# Optional: re-run staging-25 to populate load, or leave channels tracked from LOAD-001b

# SSH tunnel Grafana (from laptop)
ssh -L 3001:127.0.0.1:3000 bearhost

# Monitor loop (append evidence)
EVIDENCE=docs/pulse-extension/soak-24h-evidence.txt \
PROMETHEUS_URL=http://127.0.0.1:9090 \
INTERVAL_SEC=900 \
bash scripts/load/pulse-soak-monitor.sh
```

Record **start UTC** in `soak-24h-evidence.txt`.

## 2. During soak (24h)

Every **4 hours** (or on alert):

| Panel / query | Action |
|---------------|--------|
| `pulse_active_tracked_channels` | Must stay ≤ configured cap |
| `pulse_backfill_active_jobs` | Must stay ≤ `PULSE_MAX_BACKFILLS` |
| `up{job="analytics"}` | Must be 1 |
| p99 `pulse_golive_to_first_rollup_seconds` | Abort if >600s sustained |
| HTTP 5xx rate | Abort if >0.1/s sustained |
| `docker stats streamclone-pulse-staging-analytics` | Abort if memory >85% |

Dashboard UID: `streamclone-pulse-capacity` (see `docs/pulse-extension/pulse-metrics-runbook.md`).

## 3. Abort

See **§5** in `pulse-metrics-runbook.md`. On abort:

1. Stop monitor script
2. Capture Grafana time range + `scripts/ops-001-pulse-metrics-check.sh` output
3. Append §4.6 **FAIL** row — **do not** raise cap
4. `bash scripts/bearhost-pulse-staging-down.sh` when finished investigating

## 4. Pass criteria

- Full **24h** window without abort
- No cap breach, no OOM, no missing rollup pattern
- Evidence file completed with monitor summaries

## 5. After PASS

- Prepare `cap-001-evidence.txt` bundle
- Operator sign-off required before editing **production** `profile-bearhost-pulse.env`
- Redeploy + hosted smoke + 1–2 channel canary

## 6. Teardown

```bash
bash scripts/bearhost-pulse-staging-down.sh
```

Production tunnel (`api.streampulse.stream` :8090) is unaffected.
