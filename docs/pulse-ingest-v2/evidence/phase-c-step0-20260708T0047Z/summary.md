# Phase C Step 0 baseline summary — 2026-07-08T00:47Z UTC

**Scope:** Read-only. No deploy, restart, env edits, or data mutations.

**Evidence path (public repo interim):** `docs/pulse-ingest-v2/evidence/phase-c-step0-20260708T0047Z/`

**Ops mirror:** Copy to private `streampulse-ops/docs/deployments/ingest-core-phase-c-20260708T0047Z/` when SSH access is available.

---

## What ran

| Source | Status |
|--------|--------|
| Public API (`api.streampulse.stream`) | Collected |
| VPS SSH (`docker stats`, `redis-cli`, `df`, Prometheus) | **Blocked** — no SSH config/host resolution on agent machine |
| Postgres baseline queries | **Blocked** — hosted-data MCP unavailable; requires VPS `docker compose exec postgres psql` |
| Rollback anchors (IMAGE_TAG, env snapshot) | **Blocked** — requires streampulse-ops / VPS read-only copy |

---

## Pass / fail checklist

| Check | Result | Notes |
|-------|--------|-------|
| Hub health | **PASS** | HTTP 200; hub Cache-Control present |
| Extension health | **PASS** | `ok:true`, version `v0.3.0-rc19` |
| IRC load | **PASS** | `collectorActive=250`, `collectorMax=250` |
| Ingest flags (pre-release) | **PASS (N/A)** | `ingest` block absent — ingest-core image not deployed; treat as legacy |
| Moments Cache-Control | **FAIL** | 200 on valid `bucketT` but **no `Cache-Control` header** on prod |
| Coverage state | **WARN** | `coverage.state=degraded`, corpusPipeline degraded, deficit 25 |
| Redis rejected_connections | **UNKNOWN** | Needs VPS redis-cli |
| Postgres size/dead tuples | **UNKNOWN** | Needs VPS psql |
| Prometheus ingest_* metrics | **METRIC ABSENT PRE-RELEASE** | Expected before new image; not a failure |
| Rollback anchors saved | **UNKNOWN** | Needs VPS/ops env read |

---

## Phase C shadow readiness

**NO-GO for Phase C shadow** until blockers cleared (see step1-preconditions evidence).

**Phase D: NO-GO** (shadow not deployed yet).

---

## Next actions (ordered)

1. Operator: SSH to VPS; run full Step 0 command block + Postgres SQL; save to ops evidence bundle
2. Code: fix allowlist gate in `ingestcore/engine.go` + unit test
3. Deploy: release containing moments Cache-Control + ingest-core + allowlist fix
4. Ops: apply Docker resource limits one service at a time; wait 15–30 min between; recapture stats/logs/hub/redis delta
5. Then: deploy Phase C shadow env via streampulse-ops (legacy writer only)

## Code fixes landed (repo)

- Allowlist uses parsed IRC channel (`ingestcore/engine.go`, tests in `engine_test.go`)
- Shadow artifact rotation at 128MiB (`ingestcore/shadow.go`)
- Ops scripts: `scripts/ingest-phase-c-step0-baseline.sh`, `ingest-phase-c-preconditions.sh`, `ingest-phase-c-gates.sh`
- Env: `deploy/env/profile-hosted-ingest-core-phase-c.env.example`
