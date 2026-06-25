---
name: backfill-safety-review
description: Review VOD backfill jobs, rate limits, and capacity before enabling or widening backfill. Use when changing PulseBackfillManager, backfill API routes, job polling, or "Load missed moments" triggers.
---

# Backfill safety review

## Read first

- Sibling [`streamclone-pulse/docs/pulse-extension/live-coverage-requirements.md`](../../../../streamclone-pulse/docs/pulse-extension/live-coverage-requirements.md) § backfill
- `internal/analytics/pulse_backfill.go`, `sync_pulse_missed.go`

## Safety checklist

- [ ] Backfill requires resolvable `vodId` + Twitch archive chat — never silent no-op success
- [ ] Job states are terminal and honest (`failed`, `vod_unavailable`, not stuck `fetching_chat`)
- [ ] No unbounded concurrent backfills per channel/principal
- [ ] Extension/portal poll interval is reasonable (no tight loops hammering BFF)
- [ ] Rollups written server-side only; client never merges raw chat
- [ ] Hosted mode respects beta-key principal scoping (`pulse_hosted.go`)

## Script

```bash
python .cursor/skills/pulse/backfill-safety-review/scripts/backfill-smoke.py --login LOGIN
python .cursor/skills/pulse/backfill-safety-review/scripts/backfill-smoke.py --base https://api.streampulse.stream
```

## Block merge if

- Fake progress bars without job status backing
- Client-side rollup merge or Pulse rescoring
- Public endpoint triggers backfill without auth
