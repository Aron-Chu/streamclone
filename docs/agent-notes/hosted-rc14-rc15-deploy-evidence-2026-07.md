# rc14 + rc15 VPS deploy evidence (2026-07-05)

Operator note for **streampulse-ops** `docs/deployments/` (copy manually — private repo).

## rc14 — P1 idle admission touch

- **Tag:** `v0.3.0-rc14`
- **Change:** `TouchAdmissionObservation` on top-roster `already_tracking` / `duplicate_stream` skips
- **Phase 1:** 500 IRC cap soak — no PART/evict churn in 20m logs
- **Phase 2:** Env raised to 5000 (`PULSE_MAX_ACTIVE_CHANNELS`, `PULSE_TOP500_ADMISSION_TOP_N`, etc.)
- **Observed bug:** Hub showed `liveAdmissionTopN: 100` despite env 5000; `collectorActive` ~115

## rc15 — admission topN config clamp fix

- **Tag:** `v0.3.0-rc15`
- **PR:** [#46](https://github.com/Aron-Chu/streamclone/pull/46)
- **Root cause:** `config.Load()` reset `PULSE_TOP500_ADMISSION_TOP_N > 1000` to **100** (corpus cap leak)
- **Deploy:** `IMAGE_TAG=v0.3.0-rc15 bash /root/streampulse-ops/scripts/deploy/production-deploy.sh`
- **Env file:** `/root/streampulse-ops/env/production.local.env` — `IMAGE_TAG` + `STREAMCLONE_VERSION` → rc15

## Post-rc15 verify (streampulse-vps)

| Check | Result |
|-------|--------|
| Image | `ghcr.io/aron-chu/streamclone/analytics:v0.3.0-rc15` |
| `/v1/extension/health` version | `v0.3.0-rc15` |
| Hub `liveAdmissionTopN` | **5000** (was 100) |
| Hub `collectorActive` | **~4990** (was ~115) |
| Hub `collectorMax` | 5000 |
| IRC env | unchanged 5000 headroom |

## Notes

- 24h chart gaps from analytics recreates age out; no IRC backfill for live holes.
- Release CI smoke still fails (stack health timeout); analytics image builds OK.
- WSL `E_UNEXPECTED` fixed with `wsl --shutdown` before SSH deploy scripts.
