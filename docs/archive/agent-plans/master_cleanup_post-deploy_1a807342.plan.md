---
name: Master cleanup post-deploy
overview: "COMPLETED — Historical record + post-cleanup runbook. Do not re-run cleanup from dirty local trees. Next safe step: clean rsync of origin/master to VPS."
status: completed
completed_at: 2026-06-28
origin_master: 27a434e
---

> **Historical execution plan — do not re-run from local dirty trees.**
>
> Cleanup commits are on `origin/master` at **`27a434e`**. Prod API and schema (version **50**) are good. This file is a **completion record and runbook** for the next agent, not a pre-fix task list.

# Master cleanup — completed (Phase 2 post-deploy)

## Completion summary

| Item | Was broken | Fixed on `27a434e` |
|------|------------|-------------------|
| Compile | `PublicEmoteMaterializationRoutes` dangling in `api.go` | Removed — `8d13f4b` |
| Migrations 045–049 | Missing from repo/VPS after rsync `--delete` | Restored (reconstructed from prod schema, idempotent) — `f4361af` |
| Smoke script | `python3` on VPS; unsafe beta-key alias | jq-native + safe `set -u` alias + curl body fix — `27a434e` |
| Pulse emotes fallback | `fix/global-emotes-fallback` too large (118 commits) | Narrow branch `fix/public-emotes-overview-fallback` (`2394af3`, 2 files) pushed |

**Commits on `origin/master` (newest first):**

```
27a434e fix(ops): make pulse-hosted-boundary-smoke jq-native
f4361af chore(migrations): restore 000045-000049 chain for BearHost migrate parity
8d13f4b fix(analytics): remove dangling materialization route registration
af788cf fix(analytics): gate hosted timelines and enforce 000050 predeploy  ← prior master
```

**Prod verdict (unchanged):**

- `/v1/extension/health` → 200
- `/v1/analytics/channels/ludwig/live` → 401 unauthenticated
- `/v1/public/emotes/overview?range=7d` → 200, `state=unavailable`, `aggregateOnly=true`
- Schema `schema_migrations.version = 50`
- **IVR shadow overlay: HOLD** — do not enable

---

## Do not use these local paths for deploy/sync

| Path | Problem |
|------|---------|
| `C:\Users\Aron\twitch-7tv-clone` | Local `master` at **`af788cf`** (3 commits behind `27a434e`); dirty WIP including untracked `public_emote_materialization_status.go` and migrations 051–054 |
| `C:\Users\Aron\streamclone-deploy` | Stale at **`af788cf`**; dirty `api.go` with dangling route |
| `C:\Users\Aron\streamclone-pulse` (default checkout) | Dirty; branch-diverged; hub/extension WIP untracked |

**Always use:**

- Clean streamclone worktree at **`origin/master` (`27a434e` or newer)**, or
- Remote PR branches (e.g. `origin/fix/public-emotes-overview-fallback`) for pulse-only work

Refresh or create clean worktree:

```powershell
cd C:\Users\Aron\twitch-7tv-clone
git fetch origin
git worktree add C:\Users\Aron\streamclone-prod-sync origin/master
cd C:\Users\Aron\streamclone-prod-sync
git log -1 --oneline          # must be 27a434e or newer
git status --short            # must be empty
```

Pre-flight: confirm on disk in clean worktree only:

- `migrations/000045_*` through `migrations/000049_*` (10 files)
- `migrations/000050_stream_chat_source.{up,down}.sql`
- `internal/analytics/api.go` has **no** `PublicEmoteMaterializationRoutes` call

---

## Validation actually run (2026-06-28)

### Fresh isolated DB (not default compose volumes)

Used disposable Postgres on isolated Docker network (`scripts/tmp/migrate-smoke-isolated.sh` pattern — no host port bind):

- Full migrate chain **000001 → 000050**
- `schema_migrations.version = 50`
- Gate columns present: `analytics_minute_rollups` chat_source columns (3); `analytics_streams` chat metadata columns (4)

**Do not** use `docker compose run --rm migrate` against the default dev stack without a disposable project/volume — that can hit existing volumes and give a false signal.

### Prod read-only (no migrate writes)

- `BEARHOST_ANALYTICS_GATE_REMOTE=1 make bearhost-analytics-predeploy-gate` → **PASS**
  - `MIGRATION_000050=PASS source_columns=3`
  - `BLOCK_ANALYTICS_RECREATE=0`
  - `ANALYTICS_DEPLOY_GATE=PASS`
- `bash scripts/pulse-hosted-boundary-smoke.sh` → **PASS**
  - `PUBLIC_BOUNDARY=PASS`
  - `VOD_EXTENSION_CANARY=PASS`

**Do not** run `migrate up` on prod as part of “read-only” validation. Optional **separate approved step** after rsync: `make migrate` on VPS expecting **no change** at version 50.

### Go tests (clean worktree)

```bash
go build -o NUL ./cmd/analytics
go test ./internal/analytics/ -run 'TestGoldVODSegment|TestPublicEmote|EmoteHistory' -count=1
```

---

## Smoke script reference (shipped form)

Safe beta-key alias under `set -u` (do **not** copy the broken one-liner):

```bash
BETA_KEY="${PULSE_BETA_KEY:-}"
if [[ -z "${BETA_KEY}" && -n "${PULSE_BETA_KEYS:-}" ]]; then
  BETA_KEY="${PULSE_BETA_KEYS%%,*}"
fi
```

**Broken — never use:**

```bash
BETA_KEY="${PULSE_BETA_KEY:-${PULSE_BETA_KEYS%%,*}}"   # expands unset PULSE_BETA_KEYS under set -u
```

Emotes/VOD checks must save response body (not `curl_code` alone):

```bash
code="$(curl -sS -o "${tmp}" -w '%{http_code}' "${url}")"
```

---

## streamclone-pulse narrow fallback (separate PR)

**Branch:** `fix/public-emotes-overview-fallback` @ `2394af3`
**Diff vs `origin/master`:** exactly 2 files:

- `streampulse-web/src/lib/publicEmotesOverview.ts`
- `streampulse-web/tests/publicEmotesOverview.test.ts`

**Do not merge** `fix/global-emotes-fallback` (424 files).

**Test command** (`streampulse-web` has `test`, not `test:web`):

```bash
cd streampulse-web
npm run test -- tests/publicEmotesOverview.test.ts
```

PR may need to be opened on GitHub if not already created.

---

## Next safe operational step (required before next deploy)

Prod schema is already version 50, but VPS disk may still lack migration files **045–049** from the earlier rsync `--delete` incident. Sync repo source without analytics recreate:

```text
Use clean checkout only at origin/master@27a434e+.
Run: make bearhost-rsync
Then: BEARHOST_ANALYTICS_GATE_REMOTE=1 make bearhost-analytics-predeploy-gate
Optional: bash scripts/pulse-hosted-boundary-smoke.sh
Authenticated smoke: PULSE_BETA_KEY=<from VPS secret, redacted> bash scripts/pulse-hosted-boundary-smoke.sh
No analytics recreate unless gate/smoke prove code out of sync — ask first.
Do not run bearhost-pulse-api.sh without approval.
IVR shadow overlay: HOLD
```

Optional post-rsync read-only verify on VPS:

```bash
ls /opt/streamclone/app/migrations/00004{5,6,7,8,9}_*.sql /opt/streamclone/app/migrations/000050_*.sql | wc -l
# Expect 12
```

---

## Out of scope (future batches)

- Migrations **000051+** / full materialization status route — **Full Global Emotes** batch only
- IVR shadow / `profile-bearhost-corpus-ivr-shadow.env` — **HOLD**
- Analytics hub WIP, GlobalEmotes page, Emoteverse prototypes — separate branches
- Rolling back prod analytics container

---

## Final verdict (cleanup batch)

```text
CODE_VERDICT=master_matches_prod_deploy_path
PROD_VERDICT=unchanged_good
MIGRATION_000050=PASS
BLOCK_ANALYTICS_RECREATE=0
ANALYTICS_DEPLOY_GATE=PASS
PUBLIC_BOUNDARY=PASS
IVR_SHADOW_CANARY=HOLD
NEXT_STEP=clean_rsync_origin_master_to_vps
```

## Implementation todos (all completed)

- [x] Clean worktree at `origin/master`; commits pushed from worktree
- [x] Remove dangling route; `go build ./cmd/analytics` passes
- [x] Migrations 045–049 restored (reconstructed from prod schema)
- [x] Isolated fresh DB migrate 1→50; prod read-only gate PASS
- [x] jq smoke + safe `PULSE_BETA_KEYS` alias; smoke PASS on prod
- [x] Pulse two-file branch pushed; vitest 4/4 pass
- [x] `origin/master` pushed (`27a434e`); co-author trailers removed via rewrite
