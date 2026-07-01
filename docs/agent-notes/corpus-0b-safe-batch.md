# Corpus PR 0B safe batch — branch hygiene + live graph isolation

**Date:** 2026-07-01
**Branch:** `feat/corpus-0b-safe-batch` (from `origin/master`)

## Git hygiene report

| Check | Result | Risk | Action |
|-------|--------|------|--------|
| Current branch | `feat/corpus-0b-safe-batch` | — | Review-only corpus batch |
| Ahead/behind `origin/master` | 0 / 0 at branch point | — | Clean base |
| Uncommitted WIP on `master` | Stashed as `wip-audit-remediation-all` | Medium | Auth/MCP/boundary/console WIP excluded from this PR |
| Corpus changes isolated | Yes — live graph filter + docs only | Low | Audit remediation remains in stash |
| `Co-authored-by: Cursor` in new commits | None planned | — | Repo rule: Aron-Chu only |

### Already on `origin/master` (not re-ported)

| Commit / PR | Scope |
|-------------|-------|
| #31 `feat/corpus-0b-gold-segments` | `gold_vod_segments` durable ledger, integration gate, 0B-2 docs |
| #33 `fix(analytics): gold VOD segment ledger completion gates` | **PR 0B-3** completion semantics (`gold_vod_completion.go`, `applyGoldSegmentCompletionGate`) |

### This batch adds

1. `fix(analytics): isolate public live activity from corpus/import rollups` — SQL `chat_source` filters on hub bucket queries + Go-side hub activity guard.
2. `docs(corpus): record 0B-3 scope decision and safe-batch hygiene` — this file + hosted baseline refresh instructions.

### Explicitly excluded (remain in stash)

- `packages/analytics-console/*`
- Hosted boundary smoke / Caddy / guest-auth / MCP / CORS / pre-commit SDLC
- Gold GQL RPM capacity + segment heartbeat (audit follow-up; separate PR)
- Collector preemption tie-break

---

## PR 0B-3 scope decision

**Decision:** 0B-3 completion gating is **already shipped** on `origin/master` via PR #33. It is **not** part of this batch.

Evidence:

- `internal/analytics/gold_vod_completion.go` — `GoldVODSegmentUnresolvedSummary`, `goldVODSegmentsBlockCompletion`
- `internal/analytics/backfill_worker.go` — `applyGoldSegmentCompletionGate` before `resolveGoldBackfillOutcome`
- `docs/agent-notes/corpus-pr0b3-completion-semantics.md` — status updated to **Implemented**

**Next after this batch:** hosted canary with `GOLD_VOD_SEGMENTS_ENABLED=true` on one streampulse-vps worker (checklist §5–9 in `corpus-0b2-hosted-verify.md`). Do **not** enable flag globally until soak passes.

---

## Hosted baseline verification

**MCP:** `streamclone-hosted-data` was **unavailable** during this batch (server errored in Cursor). Re-run when MCP is healthy.

### Queries to run (SELECT only)

```sql
SELECT to_regclass('public.gold_vod_segments');

SELECT tier, status, count(*)::int
FROM backfill_jobs
GROUP BY 1, 2
ORDER BY 1, 2;

SELECT gold_status, count(*)::int
FROM top500_vod_inventory
GROUP BY 1
ORDER BY 1;

SELECT status, count(*)::int
FROM gold_vod_segments
GROUP BY 1
ORDER BY 1;
```

### Public hub leak check

```bash
curl -sS https://api.streampulse.stream/v1/public/hub | jq 'keys'
```

Expect: no `gold_vod_segments`, `lease_owner`, segment keys. `corpusPipeline` aggregate counts only.

Prior baseline (2026-07-01): migration present, **0** segment rows (flag off). See [`corpus-hosted-baseline-2026-07-01.md`](corpus-hosted-baseline-2026-07-01.md).

---

## What not to do yet

- No second worker / home PC burst / laptop corpus
- No `GOLD_VOD_SEGMENTS_ENABLED=true` in production (canary on one VPS only, after merge)
- No PR 1 identity/backoff, PR 2 gap/requeue APIs, admin UI, or proxies
- No live graph corpus mixing (this batch adds the SQL guard)
