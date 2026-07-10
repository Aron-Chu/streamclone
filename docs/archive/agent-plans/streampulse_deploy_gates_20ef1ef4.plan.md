---
name: StreamPulse deploy gates
overview: Close remaining StreamPulse prod gaps with script-enforced predeploy gates, DB-backed post-deploy canaries, and a narrow Commit B on top of 0b02ef6. Hard-block analytics recreate on missing migration 000050 only; public emotes route-not-404 with graceful degrade.
todos:
  - id: predeploy-gate
    content: Create bearhost-analytics-predeploy-gate.sh with ANALYTICS_DEPLOY_GATE, BLOCK_ANALYTICS_RECREATE, backward-compatible MIGRATION_000050 + IVR_SHADOW_CANARY lines
    status: completed
  - id: enforce-gate-in-deploy
    content: Call predeploy gate inside scripts/bearhost-pulse-api.sh before force-recreate analytics; exit non-zero on BLOCK_ANALYTICS_RECREATE=1
    status: completed
  - id: postdeploy-smoke
    content: Expand pulse-hosted-boundary-smoke.sh with DB-backed stream/VOD selection, PUBLIC_BOUNDARY, CHART_CANARY, VOD_EXTENSION_CANARY; SKIP (not PASS) when FIXTURE_SOURCE=fallback
    status: completed
  - id: fix-smoke-fixture-skip
    content: "Code fix: pulse-hosted-boundary-smoke.sh emits CHART_CANARY=SKIP and VOD_EXTENSION_CANARY=SKIP when DB fixture lookup fails (P2)"
    status: pending
  - id: fix-gate-public-emotes-warn
    content: "Code fix: bearhost-analytics-gate-checks.sh emits MIGRATION_PUBLIC_EMOTES=WARN tables=0 when both optional tables missing (P3)"
    status: pending
  - id: fix-runbook-remote-gate
    content: "Runbook fix: docs/bearhost-production.md steps 4/6 use BEARHOST_ANALYTICS_GATE_REMOTE=1 (P1)"
    status: pending
  - id: runbook-order
    content: Add mandatory deploy-order section to docs/bearhost-production.md; document script-level enforcement
    status: completed
  - id: pre-push-review
    content: Run git show --name-status 0b02ef6 review checklist before pushing Commit A
    status: completed
  - id: commit-b-narrow
    content: Stage Commit B only (hosted auth + gate scripts + docs + pulse publicEmotes fallback); push A then B when user approves
    status: completed
  - id: run-tests
    content: Run Go analytics, pulse extension, streampulse-web test suites from plan
    status: completed
  - id: prod-predploy-check
    content: "Predeploy read-only prod check from dev machine (BEARHOST_ANALYTICS_GATE_REMOTE=1); expect FAIL until migrate"
    status: completed
  - id: prod-postdeploy-smoke
    content: "After user-approved deploy, rerun remote gate + pulse-hosted-boundary-smoke; final report must show BLOCK_ANALYTICS_RECREATE=0"
    status: pending
isProject: false
---

# StreamPulse deploy gap closure plan (v2)

## Problem statement

The production risk is **not** “is hosted auth correct?” (code is done). The risk is **deploying the new analytics binary against Postgres without migration 000050**, which causes chart/VOD rollup reads to fail because the binary unconditionally selects `chat_source`, `source_confidence`, and `chat_source_detail`.

Turn scary steps into **binary gates**: schema ready → boundary closed → chart has points → VOD endpoint has indexed replay data.

---

## Auth fix — verified in code (no further code changes expected)

| Surface | Protection |
|---------|------------|
| `GET /v1/analytics/channels/{login}/live` (+ `?sparse=false`) | Hosted route group + handler guard + `writeStreamDetail` guard |
| `GET /v1/analytics/streams/{streamID}` | Same |
| `GET /v1/analytics/streams/{streamID}/replay-heatmap` | Route group |
| `GET /v1/portal/analytics/streams/{streamID}/minutes` | Portal middleware + shared `authorizeHostedStreamTimelineAccess` |
| Local desktop (`PULSE_HOSTED_MODE=false`) | Ungated branch unchanged |

Files: [`internal/analytics/api.go`](internal/analytics/api.go), [`internal/analytics/pulse_hosted.go`](internal/analytics/pulse_hosted.go), [`internal/analytics/portal_analytics_api.go`](internal/analytics/portal_analytics_api.go), tests in [`internal/analytics/hosted_analytics_auth_test.go`](internal/analytics/hosted_analytics_auth_test.go).

---

## Task 1 — Predeploy gate script

Create [`scripts/bearhost-analytics-predeploy-gate.sh`](scripts/bearhost-analytics-predeploy-gate.sh) (read-only SSH via [`scripts/lib/bearhost-ssh.sh`](scripts/lib/bearhost-ssh.sh)).

### Hard block (analytics recreate forbidden if FAIL)

| Check | Requirement |
|-------|-------------|
| Rollup source columns | `analytics_minute_rollups`: `chat_source`, `source_confidence`, `chat_source_detail` (count = 3) |
| Stream chat metadata | `analytics_streams`: `chat_state`, `chat_source`, `source_confidence`, `chat_coverage_pct` (count = 4) |

Reason string when blocked: `new binary reads migration 000050 columns`

### WARN only (do not fail gate)

| Check | Requirement |
|-------|-------------|
| Public emote tables | `public_emote_provider_hourly_rollups`, `public_emote_materialization_runs` — missing → `MIGRATION_PUBLIC_EMOTES=WARN` |
| IVR leakage | `SELECT ... WHERE chat_source='ivr'` when 000050 present — informational |

### Machine-readable output (final lines — include ALL of these)

```text
MIGRATION_000050=PASS|FAIL source_columns=N
MIGRATION_PUBLIC_EMOTES=PASS|WARN|SKIP tables=N
IVR_SHADOW_CANARY=HOLD|allowed_after_code_deploy
BLOCK_ANALYTICS_RECREATE=0|1
ANALYTICS_DEPLOY_GATE=PASS|FAIL
reason=...
```

**Backward compatibility:** [`scripts/bearhost-migration-000050-preflight.sh`](scripts/bearhost-migration-000050-preflight.sh) becomes a thin wrapper that calls the gate script and **must still emit** the legacy lines `MIGRATION_000050=...` and `IVR_SHADOW_CANARY=...` (other runbooks/scripts may grep them).

Exit code: **non-zero** when `ANALYTICS_DEPLOY_GATE=FAIL` or `BLOCK_ANALYTICS_RECREATE=1`.

Add Makefile target: `bearhost-analytics-predeploy-gate`.

### Gate mode selection (P1 — do not false-PASS prod)

The gate auto-selects **local Docker** when `streamclone-postgres-1` is reachable on the dev machine. That can print `PASS` while prod still has `source_columns=0`.

| Context | Command / env |
|---------|----------------|
| **Dev machine → prod check** | `BEARHOST_ANALYTICS_GATE_REMOTE=1 make bearhost-analytics-predeploy-gate` (forces SSH to BearHost) |
| **VPS / inside `bearhost-pulse-api.sh`** | `BEARHOST_ANALYTICS_GATE_LOCAL=1 bash scripts/bearhost-analytics-predeploy-gate.sh` (local `docker exec`, no SSH loop) |
| **Never for predeploy from laptop** | Bare `bash scripts/bearhost-analytics-predeploy-gate.sh` when local stack is up |

Implementation: [`scripts/bearhost-analytics-predeploy-gate.sh`](scripts/bearhost-analytics-predeploy-gate.sh) honors `BEARHOST_ANALYTICS_GATE_REMOTE=1` before local auto-detect.

### Public emotes table status (P3 — consistent wording)

When both optional tables are absent, emit **`MIGRATION_PUBLIC_EMOTES=WARN tables=0`** (check ran; optional tables missing). Do **not** use `SKIP` for this case — `SKIP` is reserved for “check not run” (e.g. DB unreachable), not “tables missing.”

---

## Task 2 — Enforce gate in deploy script (not docs-only)

**Primary hook:** [`scripts/bearhost-pulse-api.sh`](scripts/bearhost-pulse-api.sh) — immediately **before** line that runs:

```bash
bearhost_compose_pulse up -d --force-recreate --no-deps analytics pulse-caddy
```

Insert:

```bash
echo "==> bearhost-pulse-api: predeploy gate (migration 000050 required)"
BEARHOST_ANALYTICS_GATE_LOCAL=1 bash "${ROOT}/scripts/bearhost-analytics-predeploy-gate.sh" || {
  echo "ABORT: analytics recreate blocked — apply migration 000050 first (make migrate)" >&2
  exit 1
}
```

When run **on VPS** (via `bearhost-pulse-api-remote.sh`), `BEARHOST_ANALYTICS_GATE_LOCAL=1` uses local `docker exec streamclone-postgres-1` — no SSH loop. Do **not** set `BEARHOST_ANALYTICS_GATE_REMOTE=1` inside the deploy script.

Optional: allow override only with explicit env `BEARHOST_SKIP_ANALYTICS_DEPLOY_GATE=1` (document as break-glass; default deny).

Also note in runbook: any other path that force-recreates `analytics` (e.g. [`scripts/bearhost-pulse-redeploy-remote.sh`](scripts/bearhost-pulse-redeploy-remote.sh), [`scripts/tmp/bearhost-force-analytics-rebuild.sh`](scripts/tmp/bearhost-force-analytics-rebuild.sh)) should call `bearhost-pulse-api.sh` rather than duplicating recreate — or get the same gate call.

---

## Task 3 — Post-deploy smoke script

Update [`scripts/pulse-hosted-boundary-smoke.sh`](scripts/pulse-hosted-boundary-smoke.sh).

### DB-backed fixture selection (prefer over hardcoded IDs)

Run on VPS (or via gate SSH) **before** authenticated curls:

```sql
-- Top stream with rollups (chart canary)
SELECT s.stream_id, COUNT(r.*) AS rollup_count
FROM analytics_streams s
JOIN analytics_minute_rollups r ON r.stream_id = COALESCE(s.canonical_stream_id, s.stream_id)
GROUP BY s.stream_id
ORDER BY rollup_count DESC
LIMIT 1;

-- Top VOD with rollups (extension canary)
SELECT s.vod_id, s.stream_id, COUNT(r.*) AS rollup_count
FROM analytics_streams s
JOIN analytics_minute_rollups r ON r.stream_id = COALESCE(s.canonical_stream_id, s.stream_id)
WHERE COALESCE(s.vod_id,'') <> ''
GROUP BY s.vod_id, s.stream_id
ORDER BY rollup_count DESC
LIMIT 1;
```

Export `PULSE_SMOKE_STREAM_ID`, `PULSE_SMOKE_VOD_ID`, and rollup counts from query results.

**Hardcoded ID fallbacks** (`316860077047`, `2804592918`) are allowed **only** for Phase A unauthenticated 401 probes (route shape). They must **not** satisfy Phase B/C data-proof canaries.

### Fixture lookup failure (P2 — no false PASS)

If DB-backed fixture selection fails (SSH unavailable, SQL error, empty result), emit:

```text
FIXTURE_SOURCE=fallback|db
CHART_CANARY=SKIP
VOD_EXTENSION_CANARY=SKIP
```

When `FIXTURE_SOURCE=fallback`, Phase B/C must **SKIP** (not PASS), even if HTTP 200 — route-shape success without rollup proof is insufficient. Only `FIXTURE_SOURCE=db` with `rollup_count > 0` allows `CHART_CANARY=PASS` / `VOD_EXTENSION_CANARY=PASS`.

### Phase A — no secrets

| Probe | Expect |
|-------|--------|
| `GET /v1/extension/health` | 200, `ok:true` |
| `GET /v1/analytics/channels/ludwig/live` | **401** |
| same `?sparse=false` | **401** |
| `GET /v1/analytics/streams/{streamId}` | **401** |
| `GET /v1/public/emotes/overview?range=7d` | **not 404**; accept **200 or 503** with JSON (`aggregateOnly`, `state`, or structured `unavailable`/`error`) |

Emit: `PUBLIC_BOUNDARY=PASS|FAIL`

Do **not** claim “Full Global Emotes ready” unless 000051/000052 migrated **and** smoke returns 200 with real `aggregateOnly` aggregate data.

### Phase B — optional auth (`PULSE_BETA_KEY`)

Use DB-selected `PULSE_SMOKE_STREAM_ID`:

| Probe | Expect |
|-------|--------|
| `GET /v1/portal/analytics/streams/{id}/minutes` | 200, `.minutes | length > 0` when rollup_count > 0 |
| `GET /v1/analytics/streams/{id}` with beta header | 200 with rollups |

Emit: `CHART_CANARY=PASS|FAIL|SKIP`

### Phase C — VOD extension canary

Use DB-selected `PULSE_SMOKE_VOD_ID`:

| Probe | Expect |
|-------|--------|
| `GET /v1/extension/pulse/vods/{vodId}` | not 404; 200 JSON |
| If rollup_count > 0 | `coverageStatus` in `ready|partial|syncing`; `timeline.points | length > 0` |

Emit: `VOD_EXTENSION_CANARY=PASS|FAIL|SKIP`

---

## Task 4 — Runbook deploy order

Add to [`docs/bearhost-production.md`](docs/bearhost-production.md) — **mandatory order**:

1. **Pre-push review:** `git show --name-status 0b02ef6` — confirm scope before pushing Commit A
2. Push Commit A + Commit B (narrow)
3. `make bearhost-rsync`
4. **`BEARHOST_ANALYTICS_GATE_REMOTE=1 make bearhost-analytics-predeploy-gate`** — stop if `BLOCK_ANALYTICS_RECREATE=1` (expect FAIL on prod today)
5. On VPS: `make migrate` (000050 required; 000051/000052 optional this batch)
6. Re-run **`BEARHOST_ANALYTICS_GATE_REMOTE=1 make bearhost-analytics-predeploy-gate`** — require `ANALYTICS_DEPLOY_GATE=PASS` and `BLOCK_ANALYTICS_RECREATE=0`
7. `bash scripts/bearhost-pulse-api.sh` (uses `BEARHOST_ANALYTICS_GATE_LOCAL=1` internally before recreate)
8. `bash scripts/pulse-hosted-boundary-smoke.sh` — `PUBLIC_BOUNDARY=PASS`
9. Optional: `PULSE_BETA_KEY=... bash scripts/pulse-hosted-boundary-smoke.sh` for `CHART_CANARY` + `VOD_EXTENSION_CANARY` (require `FIXTURE_SOURCE=db` for PASS)

**Runbook sync:** [`docs/bearhost-production.md`](docs/bearhost-production.md) mandatory-order steps 4 and 6 must use the remote-gate command above, not bare `bash scripts/bearhost-analytics-predeploy-gate.sh`.

**Do-not box:** IVR shadow overlay remains HOLD until migration + artifact + zero-IVR-row checks.

---

## Task 5 — Tests (before push)

**Streamclone Go:**

```powershell
go test ./internal/analytics -run "Test(GoldIVR|CanIVROverwrite|GQLPriority|WriteGoldIVRShadowArtifact|PortalMinutesCacheKey|PortalMinutesIncludesProvisionalPeaksOnlyWhenRequested|FilterTimeline|ChatSource|BulkUpsertMinuteRollupsStampsGQLCanonical|PublicEmotesOverview|PortalChannelEmotes|HostedStream|HostedChannelLive|NonHostedChannelLive|ExtensionVodPulse)" -count=1
```

**Streamclone-pulse extension + portal:** (same as v1 plan)

---

## Commit / push strategy

### Pre-push review (Commit A)

Before pushing `0b02ef6`:

```bash
git show --name-status 0b02ef6 | less
git log -1 --format='%an %ae%n%B' 0b02ef6   # no Co-authored-by
```

Commit A is large (~155 files) and includes IVR shadow code — **safe on prod only while env-disabled** (`GOLD_IVR_ENABLED=false`, no ivr-shadow overlay merged).

### Commit A — push as-is

`0b02ef6 feat(analytics): add IVR shadow canary safety path for Gold backfill`

Includes: migration 000050, extension VOD pulse route, public emotes handler, rollup source reads.

### Commit B — narrow staging only

**Streamclone:**

- `internal/analytics/api.go`
- `internal/analytics/pulse_hosted.go`
- `internal/analytics/portal_analytics_api.go`
- `internal/analytics/hosted_analytics_auth_test.go`
- `scripts/bearhost-analytics-predeploy-gate.sh`
- `scripts/bearhost-migration-000050-preflight.sh` (wrapper)
- `scripts/lib/bearhost-analytics-gate-checks.sh` (shared SQL checks — required for clean checkout)
- `scripts/lib/bearhost-analytics-gate-checks-remote.sh` (VPS SSH entry)
- `scripts/lib/bearhost-smoke-fixtures-remote.sh` (DB stream/VOD fixture export)
- `scripts/pulse-hosted-boundary-smoke.sh`
- `scripts/bearhost-pulse-api.sh` (gate enforcement)
- `docs/bearhost-production.md`
- `docs/agent-notes/ivr-gold-prod-status.md`
- `Makefile` (gate target only)

**Do NOT stage:** 000051/000052, broad dirty tree, `.env.dev`, screenshots.

**Streamclone-pulse (Pages — separate from analytics binary):**

- `streampulse-web/src/lib/publicEmotesOverview.ts`
- `streampulse-web/tests/publicEmotesOverview.test.ts`
- `streampulse-web/tests/globalEmotesPage.test.tsx`
- `streampulse-web/prototypes/emoteverse/index.html` (optional synthetic label)

---

## Final report template (agent must use)

Emit **two** verdict fields (not one blended verdict):

| Field | Values | Meaning |
|-------|--------|---------|
| `CODE_VERDICT` | `READY_FOR_MIGRATION_THEN_ANALYTICS_DEPLOY` \| `NOT_READY` | Code + gate/smoke scripts + tests ready; prod binary state irrelevant |
| `PROD_VERDICT` | `NOT_READY_UNTIL_MIGRATE_DEPLOY_SMOKE` \| `READY_POST_SMOKE` \| `READY_FOR_IVR_SHADOW_CANARY` | Actual prod state after checks |

**`CODE_VERDICT=READY_FOR_MIGRATION_THEN_ANALYTICS_DEPLOY`** when: Commit B includes all gate helper libs, remote-gate documented, tests pass, push approved. Prod may still be on old binary.

**`PROD_VERDICT=NOT_READY_UNTIL_MIGRATE_DEPLOY_SMOKE`** when: prod gate FAIL, boundary smoke FAIL, or analytics not recreated post-migrate.

**`PROD_VERDICT=READY_FOR_IVR_SHADOW_CANARY`** only when: migration PASS, analytics deployed, corpus workers, Ludwig artifacts, zero `chat_source=ivr` rows. **Not** this deploy batch.

**Post-deploy prod report** (after migrate + recreate + smoke) must include:

```text
CODE_VERDICT=READY_FOR_MIGRATION_THEN_ANALYTICS_DEPLOY
PROD_VERDICT=READY_POST_SMOKE
BLOCK_ANALYTICS_RECREATE=0
ANALYTICS_DEPLOY_GATE=PASS
MIGRATION_000050=PASS
PUBLIC_BOUNDARY=PASS
FIXTURE_SOURCE=db
CHART_CANARY=PASS|SKIP
VOD_EXTENSION_CANARY=PASS|SKIP
new_binary_requires_000050=confirmed
```

**Current state (pre-push):**

```text
CODE_VERDICT=READY_FOR_MIGRATION_THEN_ANALYTICS_DEPLOY
PROD_VERDICT=NOT_READY_UNTIL_MIGRATE_DEPLOY_SMOKE
```

**NOT** `READY_FOR_IVR_SHADOW_CANARY`.

---

## Pre-push blockers (audit v2.1)

Do **not** push Commit B until:

1. Runbook + plan use **`BEARHOST_ANALYTICS_GATE_REMOTE=1`** for dev-machine predeploy checks (P1).
2. Commit B staging list includes all three **`scripts/lib/bearhost-*`** helper files (P1).
3. Smoke script emits **`CHART_CANARY=SKIP` / `VOD_EXTENSION_CANARY=SKIP`** when fixture lookup falls back (P2) — code fix in [`scripts/pulse-hosted-boundary-smoke.sh`](scripts/pulse-hosted-boundary-smoke.sh).
4. Gate helper emits **`MIGRATION_PUBLIC_EMOTES=WARN tables=0`** when both optional tables missing (P3) — code fix in [`scripts/lib/bearhost-analytics-gate-checks.sh`](scripts/lib/bearhost-analytics-gate-checks.sh).
5. [`docs/bearhost-production.md`](docs/bearhost-production.md) deploy-order steps 4/6 updated to remote-gate command (P1).

---

## Public emotes policy (this deploy batch)

- **Hard gate:** 000050 only (rollup reads)
- **000051/000052:** WARN in gate; not in Commit B staging unless user later opts into “Full Global Emotes”
- **Smoke:** `/v1/public/emotes/overview` not 404; 200 or 503 JSON OK
- **Frontend:** Global Emotes renders unavailable/degraded cleanly ([`publicEmotesOverview.ts`](../streamclone-pulse/streampulse-web/src/lib/publicEmotesOverview.ts))
- **Marketing honesty:** do not say “Full Global Emotes ready” without 000051/000052 + materializer + 200 aggregate data
