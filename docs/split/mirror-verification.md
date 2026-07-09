# Mirror verification — Step 7 preflight gate

**Date:** 2026-07-09
**Purpose:** Block public Streamclone deletion until private **streampulse-backend** has a verified copy of all analytics/package work.

**Rollback anchor:** `v0.3.0-rc27` (last public tag with full analytics). See private **streampulse-ops** `docs/deployments/boundary-split-rollback.md`.

---

## Mirror status (2026-07-09)

Compared trees (excluding `node_modules`):

| Path | Missing in backend | Hash diffs | Status |
|------|-------------------:|-----------:|--------|
| `cmd/analytics` | 0 | 0 | **OK** |
| `cmd/backfill` | 0 | 0 | **OK** |
| `cmd/archive` | 0 | 0 | **OK** |
| `internal/analytics` | 0 | 0 | **OK** |
| `packages/pulse-core` | 0 | 0 | **OK** (after sync) |
| `packages/pulse-charts` | 0 | 0 | **OK** (after sync) |
| `packages/analytics-console` | 0 | 0 | **OK** (after sync) |

**22 package files** were newer in public Streamclone than the initial backend scaffold; they were copied to `../streampulse-backend` on 2026-07-09. Re-run:

```powershell
# PowerShell — hash diff count (expect 0 before Step 7 delete)
$paths = @('cmd/analytics','cmd/backfill','cmd/archive','internal/analytics','packages/pulse-core','packages/pulse-charts','packages/analytics-console')
```

```bash
# Bash — quick presence check from streamclone root
test ! -d ../streampulse-backend && echo "backend checkout missing" && exit 1
for d in cmd/analytics internal/analytics packages/pulse-core; do
  diff -qr "$d" "../streampulse-backend/$d" | grep -v node_modules || true
done
```

---

## Modified / untracked in public (still on disk — do not delete yet)

Git status under legacy paths (representative):

- **Modified Go:** `internal/analytics/clip_replayforge_test.go`, `portal_analytics_api.go`, …
- **Untracked Go:** `clip_replayforge_*_test.go`, `stream_vod_read.go`, `public_api_compat_test.go`, …
- **Untracked packages:** `PastBroadcastBanner.tsx`, `SessionRecapMomentsStrip.tsx`, `momentTime.ts`, `recapMoments.ts`, …

All of the above are included in the backend mirror hash match **after sync**. Treat public copies as **frozen** until Step 7 deletion PR.

---

## Step 7 preflight checklist (this PR batch)

| # | Item | Owner |
|---|------|--------|
| 1 | Mirror verification (this doc) | **Done** |
| 2 | `scripts/check-product-boundary.sh` + pre-commit hook | **Done** |
| 3 | Track `docs/streampulse-product-boundary.md`, `deploy/smoke/test-core-routes-only.sh` | Commit in preflight PR |
| 4 | Trim install/release/load scripts (core-only surfaces) | **Batch 3 done** — strict hits **152** (was 171) |
| 5 | Move ops probes/load scripts → **streampulse-ops** | Copied; delete from public |
| 6 | Delete `docs/split/**` | **After** strict grep green (not in this PR) |
| 7 | `git rm` legacy analytics trees | **After** ops confirms backend image/digest on prod |
| 8 | Hosted health + digest evidence | Ops — `ok: true` alone is insufficient |

---

## Hosted cutover evidence required before Step 7 delete

Record in private ops (not public):

- Image tag / digest serving `https://api.streampulse.stream/v1/extension/health`
- `version` field matches promoted **streampulse-backend** build (not legacy `streamclone/*` rc23 unless explicitly pinned)
- Migration table `schema_migrations_backend` bootstrap documented (`PRE_SPLIT_MAX` candidate: **63**)

Public check only:

```bash
curl -fsS https://api.streampulse.stream/v1/extension/health
```

---

## Intentionally disposable (public only)

- `docs/split/*` — transition inventories; delete before Step 7 strict merge
- Duplicate pulse load scripts after ops copy — see `streampulse-ops/scripts/load/`
