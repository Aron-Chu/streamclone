# Degraded live collector counts — ops investigation

When the StreamPulse hub footer shows **Live tracking: N / M channels · degraded** (formerly “Corpus pipeline”), the portal is surfacing honest backend admission state — not a UI bug.

## What to check (streampulse-ops)

1. **Roster vs collector cap** — compare Top-N roster size to `collectorMax` / admission env on the hosted analytics worker.
2. **Worker restarts** — IRC collector pods restarting leave gaps in `analytics_minute_rollups` until channels re-admit.
3. **Admission flags** — query roster rows for `admission_disabled`, `capacity_blocked`, `warming`, `metadata_only`, `viewer_only`.
4. **Metadata-only channels** — Helix viewer samples without IRC still write viewer-only minute rows; they will not produce chat peaks.
5. **Idle churn (P1)** — if admission skips refresh on `duplicate_stream` / `already_tracking`, steady-state tracks can PART every ~15m and re-JOIN on the next cycle. Fix: `TouchAdmissionObservation` on both outcomes ([`top-roster-idle-churn-p1-2026-07.md`](top-roster-idle-churn-p1-2026-07.md)). Watch analytics logs for repeating PART on the same login while hub still lists the channel in the live pool.

## Portal expectation

- Chart gaps with zero-filled buckets mean **no rollup row** for that minute across the pool — fix density/admission before client-side interpolation.
- Per-point `hasChatRollup` / `hasViewerRollup` on `/v1/public/hub` activity lets the chart break chat vs viewer series independently.

## Related code

- Collector flush: `internal/analytics/collector.go` → `UpsertMinuteRollup`
- Hub overlay seam: `internal/analytics/hub_overview.go` → `overlayRecentPoolHubActivity`
