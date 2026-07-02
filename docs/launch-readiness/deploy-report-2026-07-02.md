# Launch-readiness deploy report — 2026-07-02

Operator/agent rollout for `docs/launch-readiness/tasks.md` (P0 LB-01..LB-06, P1 T2-*, P2 T3-*, P3 H-04).

## Executive summary

| Phase | Status | Notes |
|-------|--------|-------|
| Commits (streamclone) | **Done** | 5 Conventional Commits pushed to `origin/master` |
| Commits (streamclone-pulse) | **Partial** | 3 commits on local `fix/public-emotes-overview-fallback`; **not** on `master` |
| Push (pulse) | **Blocked** | Branch diverged from remote; merge to `master` hit 40+ conflicts |
| Backend VPS deploy | **Blocked** | `ssh root@23.173.152.156` → `Permission denied (publickey)` |
| Cloudflare Pages deploy | **Blocked** | `CLOUDFLARE_API_TOKEN` not set in agent environment |
| Production truth plane | **Still red** | Hosted probes fail on metadata staleness and gold queue age (pre-deploy build) |

**Bottom line:** Launch-readiness code is committed and pushed for the **backend repo**. Production has **not** been redeployed; public hub still serves the old binary and omits new truth fields.

---

## Commits pushed — `Aron-Chu/streamclone` (`master`)

| Hash | Message |
|------|---------|
| `a2e0bb1` | feat(analytics): launch-readiness truth plane and corpus scaling |
| `54d9198` | feat(deploy): forward launch admission and metadata env on VPS |
| `f2edd9f` | feat(scripts): add hosted launch probes and deploy safety checks |
| `53fb6c3` | feat(analytics-console): restore exports for portal deploy gates |
| `fc9629b` | docs: add launch-readiness task ledger and operator notes |

Author on all commits: `Aron-Chu <aroncloudchu@gmail.com>` (no co-author trailers).

### Intentionally left uncommitted (streamclone)

Agent/tooling and non-launch WIP remain dirty, including:

- `.github/agents/*`, `AGENTS.md`, `Makefile`, MCP/codex examples
- Emote R2 pipeline: `cmd/emote`, `internal/emote/*`, `docs/storage/emotes-r2-migration.md`, R2 storage scripts
- BearHost worker overlay: `deploy/docker-compose.bearhost-worker.yml`, `scripts/bearhost-worker.sh`
- Hosted-data MCP scripts, integration tests, workspace docs

---

## Commits local — `Aron-Chu/streamclone-pulse`

On branch `fix/public-emotes-overview-fallback` (local only; push rejected — diverged from remote):

| Hash | Message |
|------|---------|
| `00a5f0a` | feat(portal): strengthen pages deploy prod local gates |
| `4608028` | feat(portal): launch-readiness hub truth and portal resilience |
| `3ca4345` | docs(portal): document analytics hub source contracts |

### Pulse repo blockers

1. **`git push origin fix/public-emotes-overview-fallback`** rejected (non-fast-forward; remote has 1 commit not in local branch).
2. **`git merge fix/public-emotes-overview-fallback` into `master`** after `git pull origin master` produced **40+ merge conflicts** in `streampulse-web/` (parallel hub WIP on remote `master` vs local launch branch).
3. Extension WIP (`src/`, `manifest.json`, overlay UI) deliberately **not** committed.

### Intentionally left uncommitted (streamclone-pulse)

- Extension overlay and dev scripts under `src/`, `scripts/dev-extension.mjs`, etc.
- Design evidence PNGs, pulse-extension evidence/fixtures, repomix config
- `streampulse-web/jynxzi_*.json`, `streampulse-web/firefox-review/`, `tests/lighthouse-report.json`
- Stashed: `.github/workflows/ci.yml`, `docs/CONTEXT.md`, `AGENTS.md`, `README.md`, `.gitignore` (`wip-non-portal` stash)

---

## Phase 2 — Deploy (not completed)

### Backend (streampulse-vps `23.173.152.156`)

**Attempt:** `ssh -o BatchMode=yes root@23.173.152.156`

**Result:** `Permission denied (publickey)`

**Operator steps:**

1. Ensure SSH key configured (`streampulse_vps_resolve_worker_key` in deploy script — typically `~/.ssh/id_ed25519` or env `WORKER_KEY`).
2. On VPS, set `deploy/env/profile-streampulse-vps-production.local.env` (from example) with at least:
   - `PULSE_TOP500_ADMISSION_ENABLED=true`
   - `PULSE_TOP500_ADMISSION_TOP_N=100`
   - `TOP500_METADATA_ENABLED=true`
   - `TOP500_METADATA_WRITE_ENABLED=true`
   - `TOP500_METADATA_DRY_RUN=false`
   - `PULSE_MAX_ACTIVE_CHANNELS=200`
   - `CORPUS_TARGET_TOP_N=100` (or launch target)
3. From streamclone checkout: `bash scripts/test-rollup-index-migrations.sh`
4. Deploy: `bash scripts/streampulse-vps-production-deploy.sh`
5. Apply migrations **000059**, **000060** (CONCURRENTLY — one file at a time), then **000061**.
6. Verify: `\d analytics_minute_peaks` (or equivalent read-only query).

### Portal (Cloudflare Pages)

**Attempt:** `npm run pages:deploy:prod` in `streamclone-pulse/streampulse-web`

**Local gates:** **PASS** — `tsc`, SPA route check, link check, `vite build`, `check-backend-url`

**Stop point:** `pages:deploy:prod requires CLOUDFLARE_API_TOKEN`

**Operator steps:**

1. Resolve pulse `master` vs `fix/public-emotes-overview-fallback` (merge or fast-forward strategy).
2. Export `CLOUDFLARE_API_TOKEN` (and optional `CLOUDFLARE_ACCOUNT_ID`).
3. `cd streampulse-web && npm run pages:deploy:prod`

---

## Phase 3 — Production verification

### Hosted launch probes (read-only, pre-deploy build still live)

```text
bash scripts/hosted-launch-probes.sh
==> Hosted launch probes: https://api.streampulse.stream activityWindow=30m
hub: state=critical liveAdmission=False collector=0/50 tracking=0 live=18 metadataStale=18
gold: queued=63 running=1 failed=70 oldestQueuedSeconds=380483
FAIL: metadata stale ratio 18/18 exceeds 0.25
FAIL: gold oldestQueuedSeconds 380483 exceeds 172800
```

### Public hub snapshot (still old API)

After commits, **before VPS redeploy**:

- `corpusPipeline.state`: `critical`
- `collectorActive`: `0`
- `collectorMax`: `50` (probe) / `200` (24h hub query earlier)
- `liveAdmissionEnabled`: **absent / false** (new fields not in deployed binary)
- `metadataSampledAgoSeconds`: **absent**
- `roster.admissionFeatureDisabled`: **absent**
- `roster.metadataStale`: `18`, `admissionDisabled`: `18`
- `/v1/public/stats` → `momentsDetected: 0` (cannot confirm peak-only semantics until new backend is live)

### Portal deploy gates (local)

All pre-Cloudflare checks pass; deploy blocked only on missing token.

---

## Remaining gaps (owners)

| ID | Item | Owner | Status |
|----|------|-------|--------|
| **Deploy** | VPS SSH + `streampulse-vps-production-deploy.sh` | Operator | Blocked — no SSH key in agent env |
| **Deploy** | Migrations 000059–000061 on production Postgres | Operator | Not run |
| **Deploy** | Cloudflare Pages with `CLOUDFLARE_API_TOKEN` | Operator | Blocked — token missing |
| **Git** | Merge/push streamclone-pulse portal commits to `master` | Operator | 40+ conflicts with remote master |
| T3-04 | CDN emote cutover (`cdn.streampulse.stream` / R2) | Operator | Code/docs only; DNS not cut |
| H-04 | Execute `scripts/cloudflared-tunnel-token-rotate.sh` | Operator | Script promoted; not executed |
| H-01/H-02 | Backup restore / tunnel-flip drill | Operator | Not run |
| BearHost | Rollback soak (`141.11.243.103`) | Operator | No SSH verified this session |
| Tests | React `act(...)` warnings in `usePublicHubData.test.tsx` | Engineering | Pre-existing; tests pass |
| Review | Collector eviction tie-break + test fixture changed together | Engineering | Flag for human review — comparator now uses `addedAt` + login; test encodes age invariant |

---

## Recommended operator sequence (single session)

1. **Pulse git:** Reconcile `master` with `fix/public-emotes-overview-fallback` (prefer merge with launch branch winning hub files, or deploy from local branch via Pages CLI without merging).
2. **Streamclone:** `git pull` on VPS via deploy script after SSH works.
3. **Env:** Confirm production `.local.env` admission/metadata/collector caps.
4. **Migrate:** 000059 → 000060 → 000061 with CONCURRENTLY discipline.
5. **Deploy backend:** `bash scripts/streampulse-vps-production-deploy.sh`
6. **Wait ~10m:** `collectorActive` should rise; `corpusPipeline.state` should leave `critical`.
7. **Deploy portal:** `npm run pages:deploy:prod` with Cloudflare token.
8. **Verify:** `hosted-launch-probes.sh`, `pulse-hosted-boundary-smoke.sh`, manual `/analytics` smoke.
9. **Re-run this report's probe section** and append AFTER values.

---

## Pre-commit note (streamclone)

Partial analytics commits fail pre-commit `go vet` when unstaged sibling files are stashed. The merged analytics commit (`a2e0bb1`) includes the full `internal/analytics` launch surface plus `internal/emoteimage` so hooks pass. Slice granularity was reduced accordingly.
