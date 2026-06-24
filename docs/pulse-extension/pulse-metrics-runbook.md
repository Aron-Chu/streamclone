# Pulse metrics — Grafana dashboard & LOAD-001 gate (OPS-001)

Status: operator runbook (Streamclone backend). Dashboard UID: **`streamclone-pulse-capacity`**.

---

## 1. Open the dashboard

Grafana on BearHost binds **localhost only** (`127.0.0.1:3000`). Use an SSH tunnel from your dev machine:

```bash
# From streamclone repo root (WSL or Linux)
make bearhost-grafana-tunnel
# or: bash scripts/bearhost-grafana-tunnel.sh
```

Then open:

```text
http://localhost:3001/d/streamclone-pulse-capacity/streampulse-pulse-capacity-and-coverage
```

Login: `admin` / value of `GRAFANA_ADMIN_PASSWORD` (default in compose: `streampulse`).

**Sync dashboard JSON to VPS** after repo changes:

```bash
bash scripts/bearhost-grafana-sync-remote.sh
```

**Verify Prometheus queries** (on VPS or via tunnel to `:9090`):

```bash
bash scripts/ops-001-pulse-metrics-check.sh
```

---

## 2. Confirm Grafana is not public

| Check | Expected |
|-------|----------|
| `curl -sf --max-time 5 https://grafana.streampulse.stream/` | **Fails** (timeout / connection refused / no route) until Cloudflare Access is configured |
| VPS `docker compose … ps grafana-obs` | Port mapping `127.0.0.1:3000->3000` only |
| Cloudflare Tunnel public hostnames | **`api.streampulse.stream` only** for Pulse API — not Grafana |

Cloudflare Access on `grafana.streampulse.stream` is planned ([INFRA-004 decision](https://github.com/Aron-Chu/streamclone-pulse/blob/master/docs/website-portal/infra-004-admin-access-decision.md)); until Batch S implements it, treat Grafana as **operator-local via SSH tunnel only**.

---

## 3. Dashboard panels (OPS-001)

| Panel | Prometheus metric | Notes |
|-------|-------------------|--------|
| Active tracked channels | `pulse_active_tracked_channels` | Compare to cap line **10** (`PULSE_MAX_ACTIVE_CHANNELS`) |
| Active backfill jobs | `pulse_backfill_active_jobs` | Compare to cap line **1** (`PULSE_MAX_BACKFILLS`) |
| Go-live detections | `pulse_golive_detected_total{source}` | Counter rate by source class |
| Go-live → first rollup | `pulse_golive_to_first_rollup_seconds` | Histogram p95/p99 |
| Tracked from start | `pulse_tracked_from_start_total` | First rollup within 120s |
| Coverage start offset | `pulse_coverage_start_offset_seconds` | Histogram p50/p95 |
| Scrape up | `up{job="analytics"}` | Must stay **1** |
| HTTP errors | `http_requests_total{job="analytics",status=~"4..\|5.."}` | 4xx/5xx rates |

**Missing / not on this dashboard (follow-up):**

- Container memory / OOM — no node_exporter on BearHost Pulse profile; use `docker stats analytics` manually during LOAD-001.
- Per-route BFF latency histogram — use logs or add a dedicated metric in a future ops task.

---

## 4. What “safe to start LOAD-001” means

LOAD-001 (25-channel synthetic harness) may start only when **all** of:

1. **OPS-001 PASS** — this dashboard loads and shows live gauge values (even if counters are zero).
2. **Batch Q deploy stable** — hosted smoke PASS (`version=v0.3.0-rc4`), cap still **10** in `profile-bearhost-pulse.env`.
3. **Scrape healthy** — `up{job="analytics"} == 1` for ≥24h without gaps (check time series panel).
4. **No sustained 5xx** — `rate(http_requests_total{status=~"5.."}[5m])` near zero during normal beta traffic.
5. **Explicit operator approval** — LOAD-001 runs on staging/isolated profile, not production cap raise.

**Do not** set `PULSE_MAX_ACTIVE_CHANNELS=25` until **CAP-001** after LOAD-001 soak evidence.

---

## 5. LOAD-001 abort thresholds

Stop the harness immediately if any condition persists **>5 minutes**:

| Signal | Threshold |
|--------|-----------|
| HTTP 5xx | Sustained `>0.1/s` on analytics or smoke script failures |
| Active channels | `pulse_active_tracked_channels` **>** configured cap (10) |
| Backfill concurrency | `pulse_backfill_active_jobs` **>** `PULSE_MAX_BACKFILLS` (1) |
| Scrape / backend | `up{job="analytics"} == 0` |
| Missing rollups | Go-live detections without first-rollup histogram samples over expected window (check logs + `pulse_golive_to_first_rollup_seconds` panel) |
| Latency SLO | p99 go-live → first rollup **>** 600s during harness |
| Memory pressure | Analytics container **>85%** of cgroup limit or OOM kill (manual `docker stats`) |

After abort: capture dashboard time range, `ops-001-pulse-metrics-check.sh` output, and `/v1/extension/health` — append to §4.6 ledger; do not raise cap.

---

## 6. LOAD-001 harness

Script: `scripts/load/pulse-25-channel-harness.sh` (Python core: `scripts/load/pulse_harness.py`).

| Mode | Purpose |
|------|---------|
| `dry-run` | Target/roster/beta-key/health validation — **no watch mutations** |
| `smoke` | 2–5 channels, staggered watch + poll (default 3) |
| `staging-25` | 25 channels — **isolated/staging/local only**; requires `PULSE_LOAD_STAGING_CONFIRM=1` |

```bash
# Dry-run against public API (safe)
PULSE_LOAD_TARGET=https://api.streampulse.stream PULSE_LOAD_MODE=dry-run \
  bash scripts/load/pulse-25-channel-harness.sh

# Smoke on legacy-rollback-host (operator)
bash scripts/load/pulse-load-smoke-vps.sh
```

Grafana during smoke: `http://localhost:3001/d/streamclone-pulse-capacity` (SSH tunnel).

Evidence files: [`ops-001-evidence.txt`](./ops-001-evidence.txt), [`load-001-dry-run-evidence.txt`](./load-001-dry-run-evidence.txt), [`load-001-smoke-evidence.txt`](./load-001-smoke-evidence.txt).
