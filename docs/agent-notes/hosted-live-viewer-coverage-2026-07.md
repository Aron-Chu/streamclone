# Hosted live viewer coverage (2026-07)

## Problem

Live extension/portal charts can show chat from minute ~2 while viewer samples appear at ~5+ because:

1. Top-N IRC admission runs on a 60s cycle (cap 250).
2. Helix viewer samples were held in-memory until the minute boundary flushed to Postgres.
3. UI reads DB rollups only — open-minute viewer samples were invisible to 30s extension polls.

## Code fixes (streamclone analytics image)

| Change | File | Effect |
|--------|------|--------|
| `bindStreamIDNow` on IRC admit | `internal/analytics/collector.go` | Helix bind + first viewer sample immediately |
| `flushOpenMinuteToStore` | `collector.go` | Writes current-minute rollup to Postgres (~10s rate limit) |
| `TouchAdmissionObservation` bind | `collector.go` | Re-bind when already tracking but `currentStreamID` empty |
| `viewerStartOffsetSeconds` | `internal/analytics/extension_api.go` | Pulse payload honesty field |

## Ops knobs (hosted overlay)

| Knob | Default | Stream-start recommendation |
|------|---------|----------------------------|
| `PULSE_TOP500_ADMISSION_INTERVAL` | `60s` | Optional `30s` for faster top-N churn |
| `ANALYTICS_POLL_INTERVAL` | `15s` | Keep — bind covers admit gap |
| Protect / always-track | per channel | **Required** for minute-0 channels outside top-N |
| `TIER0_SAMPLE_INTERVAL` | `45s` | Keep — metadata roster, not IRC chat |

See `deploy/env/profile-hosted-pulse-live-250.env.example`.

## Deploy

1. Build/push analytics image from streamclone `release/top-live-irc-admission` tag **`v0.3.0-rc16`** (pinned `IMAGE_TAG` in streampulse-ops).
2. Run migrate rc16 forward (`000062`/`000063`), then recreate analytics container.
3. **Break-glass:** keep metadata/video/chat/emote/frontend on rc15 (or rc13) — analytics + migrate only.
4. **Protect** a test channel **before** go-live; start a new stream; probe within 2–3 minutes.

## Deploy evidence (rc16)

| Field | Value |
|-------|-------|
| Tag | `v0.3.0-rc16` |
| Commit | `ff161ef` on `release/top-live-irc-admission` |
| Scope | `internal/analytics/**`, migrations 000062–063, live viewer flush |
| Break-glass | analytics + analytics-workers + migrate **rc16**; metadata/emote **rc15**; `production.local.env` `IMAGE_TAG` unchanged (**rc15**) |
| Migrate | `62/u auto_clipper_candidates`, `63/u auto_clipper_replayforge_jobs` (2026-07-05) |
| Deploy script | `scripts/tmp/vps-rc16-analytics-breakglass.sh` → VPS `/tmp/` |

### Version sanity (2026-07-05)

| Check | Result |
|-------|--------|
| `analytics` container image | `ghcr.io/aron-chu/streamclone/analytics:v0.3.0-rc16` |
| `analytics-workers` container image | `ghcr.io/aron-chu/streamclone/analytics:v0.3.0-rc16` |
| `metadata` / `emote` images | `v0.3.0-rc15` (unchanged) |
| `/v1/extension/health` `version` | **`v0.3.0-rc15`** — reads `STREAMCLONE_VERSION` env (break-glass leaves env at rc15); use container image tag as deploy truth |
| Hub `liveAdmissionTopN` | **5000** |
| Hub `collectorActive` | ~600 immediately post-recreate; re-admitting toward ~5000 |

### Rollup probe (post-deploy, top-roster admission)

Channel **`enzzai`** — live stream admitted ~3 min after rc16 analytics recreate (top-roster, not formal Protect):

| Metric | Value |
|--------|-------|
| `tracking` | `true` |
| `isLive` | `true` |
| `coverageStartOffsetSeconds` | 180 |
| `viewerStartOffsetSeconds` | 180 |
| First chat rollup | offset **180**, `chatCount=113`, `viewerCount=287` |
| Rollup delta (chat − viewer start) | **0s** — pass (≤60s) |
| `viewerStartOffsetSeconds` in JSON | present as **180** (viewers did not lag chat) |

**Pass:** first minute with chat includes viewers; honesty offsets align.

**Follow-up:** formal offline-Protect → new-stream minute-0 test on an operator-owned channel when convenient.

**Waiver (break-glass closure):** top-roster admission evidence (`enzzai`, rollup delta 0s) is sufficient for rc16 structural validation; outside-pool Protect path remains follow-up only.

### Rollback

Pin analytics + workers back to **`v0.3.0-rc15`** app image only; do **not** migrate down. Avoid rc13 — loses rc15 admission topN clamp (~115 vs ~4990 active).

## Verification probe

```powershell
$login = 'your_test_channel'
$r = Invoke-RestMethod "https://api.streampulse.stream/v1/extension/pulse/channels/$login"
$r.coverageStartOffsetSeconds
$r.viewerStartOffsetSeconds
$r.rollups | Select-Object -First 8 offsetSeconds, chatCount, viewerCount
```

**Pass (after deploy + Protect):**

- First rollup with `chatCount > 0` has `viewerCount > 0` (or within one minute).
- `viewerStartOffsetSeconds` ≤ `coverageStartOffsetSeconds + 60` when both align.
- Extension live chart shows cyan viewer band when rollups include viewers.
- Portal live console shows sky banner only when viewers start materially after chat.

**Pre-deploy baseline:** hosted may still return rollups without aligned viewers until the new image ships.
