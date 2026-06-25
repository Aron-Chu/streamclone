---
name: pulse-live-coverage-review
description: Architecture and algorithm review for Pulse live coverage, VOD backfill from stream start, Protect channel, and BearHost hosted tracking. Use before scaling collectors, changing coverage state machines, or planning go-live detection.
---

# Pulse live coverage — architecture review

## Mode

**Review by default** — no code edits unless the user asks. Be blunt; challenge overpromises.

## Read first (order)

1. Sibling [`streamclone-pulse/docs/pulse-extension/live-coverage-requirements.md`](../../../../streamclone-pulse/docs/pulse-extension/live-coverage-requirements.md) — canonical requirements (583 lines)
2. [`docs/roadmapping.md`](../../../../docs/roadmapping.md) — phased delivery R0–R6
3. [`docs/tools.md`](../../../../docs/tools.md) — data source tiers (core / enrichment / risky)
4. [`docs/finalplan.md`](../../../../docs/finalplan.md) — archive phases A–D status
5. Backend: `internal/analytics/pulse_coverage.go`, `pulse_backfill.go`, `sync_pulse_missed.go`, `collector.go`, `extension_api.go`
6. Env: `deploy/env/profile-bearhost-pulse.env`

Portal/extension contract: sibling `streamclone-pulse/src/background/api.ts`, `src/ui/CoverageCard.tsx`.

## Companion skills

Load when drilling into a sub-area:

| Skill | When |
|-------|------|
| `coverage-triage` | UX vs backend coverage truth |
| `backfill-safety-review` | VOD backfill jobs, rate limits |
| `capacity-governor-review` | Caps, always-track, BearHost scale |
| `api-contract-drift-check` | BFF ↔ extension ↔ pulse-core fields |
| `analytics-sync` | GQL VOD chat fetch, rollup merge |
| `context-retrieval` | Codegraph before broad grep |

## MCP tools (use before grep)

| Server | Tools |
|--------|-------|
| `streamclone-codegraph` | `search_symbols`, `get_ast_chunk`, `get_blast_radius`, `get_call_chain` |
| `streamclone-data` | `postgres_query` (SELECT), `redis_get`, `data_status` |
| `streamclone-stack` | `stack_health`, `compose_logs(analytics)`, `stack_ports` |

**Seed symbols:** `computePulseCoverage`, `SyncPulseMissedChat`, `PulseBackfillManager`, `Collector`, `extensionPulseChannel`, `enrichExtensionCoverage`

**Skip for review:** Playwright, figma-bridge (unless UI pixel task).

## Product line (non-negotiable)

> Pulse tracks live from when tracking begins. If you join late, earlier chat can only be filled when Twitch has a VOD chat replay. Protect a channel so future streams are tracked from 00:00.

**Path A — Live tracking:** IRC while live; no VOD required; full-from-start only if join ≤ ~120s after go-live.

**Path B — VOD backfill:** GQL VideoComments for missing prefix; requires `vodId`; impossible if no/deleted/private VOD.

Never conflate player DVR seek with analytics backfill.

## Probes

```bash
# Local (stack up)
curl -s http://localhost:8090/v1/extension/health
curl -s "http://localhost:8090/v1/extension/pulse/channels/CHANNEL?window=full" | head -c 2000

# Hosted (no secrets in output)
curl -s https://api.streampulse.stream/v1/extension/health

# Scripts
python .cursor/skills/pulse/backfill-safety-review/scripts/backfill-smoke.py --login CHANNEL
python .cursor/skills/pulse/api-contract-drift-check/scripts/contract-keys.py
```

## Two backfill systems (do not confuse)

| System | Code | Purpose |
|--------|------|---------|
| **Pulse extension backfill** | `pulse_backfill.go`, `sync_pulse_missed.go` | User-triggered missing prefix |
| **Archive gold/silver** | `backfill_worker.go`, `gold_enqueuer.go` | Corpus tier; disabled on BearHost Pulse profile |

## Output format

1. Executive summary (5 bullets)
2. Reality check (% built, critical gaps)
3. Build sequence (4–8 weeks)
4. Algorithm / optimization proposals (ranked by ROI)
5. Architecture risks + mitigations
6. Capacity model (BearHost 8GB, caps in profile-bearhost-pulse.env)
7. Top 10 actionable tickets (P0/P1/P2)

## Block bad advice

- Extension GQL/DOM as primary VOD source
- 500 roster = 500 simultaneous IRC joins
- Client-side Pulse scoring or rollup merge
- Recovering arbitrary deleted/no-VOD chat without prior live capture
- Fake backfill progress without job status
