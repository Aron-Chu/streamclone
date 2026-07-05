# Top-roster idle churn — P1 fix (2026-07)

| | |
|---|---|
| **Risk** | Periodic PART → JOIN on steady-state top-roster IRC tracks after ~15m idle TTL |
| **Blocker for** | IRC cap scale **500 → 750** (not 1000/2500/5000) |
| **Fix status** | Branch `fix/top-roster-idle-admission-touch` → tag `v0.3.0-rc14` |

## Root cause

Top-roster admission poller (`top500_priority_watch.go`) skips `WatchWithPriority` on channels already in the pool. Two skip outcomes never refreshed `lastViewedAt`:

| Outcome | When | Steady state? |
|---------|------|----------------|
| `already_tracking` | Login in cycle snapshot, stream ID not yet matched | Early after JOIN |
| `duplicate_stream` | `TrackedStreamID(login) == row.StreamID` | **Yes** — normal after `pollOnce` sets `currentStreamID` |

`evictIdleChannels` (default 15m TTL) then PARTs; next admission cycle JOINs again → churn waves at higher caps.

## Fix (minimal)

`Collector.TouchAdmissionObservation(login)` — sets `lastViewedAt = now` only. Does **not** set `poolAlwaysTrack`, principal refs, or global protected.

Called from admission on **both** `duplicate_stream` and `already_tracking` before metrics/continue.

## Tests

- `collector_admission_idle_test.go` — touch semantics, poller `already_tracking`
- `top500_live_admission_test.go` — `TestTop500PriorityWatchDuplicateStreamRefreshesIdleClock`

```bash
go test ./internal/analytics/... -run 'DuplicateStream|TopRoster|TouchAdmission|Idle' -count=1
```

## Deploy / scale order

1. Merge/tag `v0.3.0-rc14` from `fix/top-roster-idle-admission-touch` (P1-only — no rollup batching).
2. Deploy analytics on hosted (`streampulse-ops`, rc13 → rc14).
3. Soak at **500** ≥20m — no repeating PART on same login every ~15m.
4. Then env canary **750** — see streampulse-ops `PREREQ-top-roster-idle-p1-before-750.md`.

## Related

- [`degraded-collector-investigation-2026-07.md`](degraded-collector-investigation-2026-07.md)
- Code: `top500_priority_watch.go`, `collector.go` → `TouchAdmissionObservation`
