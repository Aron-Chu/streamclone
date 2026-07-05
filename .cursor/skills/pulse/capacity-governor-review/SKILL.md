---
name: capacity-governor-review
description: Review tracking pool caps, always-track eviction, rate limits, and hosted beta capacity before scaling collectors or watchlists. Use when changing always-track, watchlist size, Helix polling, or BearHost pulse profile env.
---

# Capacity governor review

## Read first

- Sibling [`streamclone-pulse/docs/website-portal/design.md`](../../../../streamclone-pulse/docs/website-portal/design.md) — tracking pool, principal scoping
- `internal/analytics/pulse_hosted.go`, `deploy/env/profile-bearhost-pulse.env`

## Checklist

- [ ] Always-track entries respect pool cap and idle eviction policy
- [ ] Top-roster admission refreshes idle on steady-state skip outcomes (`duplicate_stream`, `already_tracking`) via `TouchAdmissionObservation` — see [`docs/agent-notes/top-roster-idle-churn-p1-2026-07.md`](../../../docs/agent-notes/top-roster-idle-churn-p1-2026-07.md) before raising IRC cap
- [ ] Beta-key principals cannot unboundedly expand watchlists (`PULSE_MAX_CHANNELS_PER_PRINCIPAL`)
- [ ] Go-live detector SLA documented (Helix poll vs EventSub)
- [ ] Backfill concurrency capped per host (`PULSE_MAX_BACKFILLS`)
- [ ] Public stats/status endpoints are aggregate-only and cached
- [ ] No new unauthenticated heavy endpoints
- [ ] 500 roster (`BRONZE_TOP_N`) ≠ simultaneous IRC joins (`MAX_CONCURRENT_TRACKED_CHANNELS`)

## Probes

```bash
curl -s http://localhost:8090/v1/extension/health
curl -s https://api.streampulse.stream/v1/extension/health
grep -E 'PULSE_|MAX_CONCURRENT' deploy/env/profile-bearhost-pulse.env
```

## Escalate when

Tunnel, Caddy route, or compose profile changes touch BearHost deployment.
