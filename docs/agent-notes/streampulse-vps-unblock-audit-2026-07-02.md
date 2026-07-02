# StreamPulse VPS migration unblock audit — 2026-07-02 (post script fixes)

**Branch:** `fix/vps-migration-hardening`
**Scope:** Read-only hosted/BearHost checks + deploy blockers fixed locally. No DNS cutover, no pg_restore, no volume deletes.

---

## Git / branch

| Item | State |
|------|--------|
| Branch | `fix/vps-migration-hardening` |
| HEAD (before this commit batch) | `3395b03` docs(ops): record StreamPulse VPS migration baseline |
| Prior canary commits | `39e3d1f`, `bdd2902`, `ad127bf`, `2660a3a`, `7197575` |
| Unrelated dirty work | cleared (line-ending-only pulse lease files reset) |
| Local-only helpers | `scripts/tmp/` left untracked |

---

## Hosted API (BearHost still serving `api.streampulse.stream`)

| Check | Result |
|-------|--------|
| `GET /v1/extension/health` | **200** ~336–402ms |
| `GET /v1/public/hub?activityWindow=30m` | **200** ~308–1007ms |
| `GET /v1/public/hub?activityWindow=7d` | **200** ~344–387ms |

Hub perf blocker remains **cleared**.

---

## BearHost Postgres (read-only)

| Check | Result |
|-------|--------|
| `schema_migrations` | version **58**, dirty **false** |
| `gold_vod_segments` done | 6408 |
| queued | 1373 |
| running | 24 |
| skipped | 5633 |
| stale running (`lease_expires_at < now()`) | **24** |
| false-done (exact query via `backfill_job_id`) | **0** |
| `public_emote_provider_hourly_rollups` | **not present** on BearHost (Atlas/materializer tables retired or never on prod) |

Stale reclaim still blocked on current topology: worker-only VPS cannot reach BearHost Postgres on tailnet.

---

## VPS access / stacks

| Check | Result |
|-------|--------|
| SSH `root@23.173.152.156` from Windows | **Permission denied** (no `~/.ssh/id_ed25519`; bearhost key rejected for root) |
| SSH via WSL | not confirmed this session — use `/home/aron/.ssh/id_ed25519` from WSL/bash |
| Old worker-only stack | **unknown** until VPS SSH works; deploy preflight now blocks if `streampulse-analytics-workers` / `streampulse-scraper` exist |

---

## Blockers fixed in this batch

1. **Restore compose project mismatch** — `streampulse-vps-production-restore.sh` now uses `streamclone-production` project + same compose stack as deploy; resolves postgres by service id; requires `STREAMPULSE_PG_RESTORE_CONFIRM=YES_I_HAVE_PROD_BACKUP`; refuses if analytics/workers running.
2. **Deploy used GHCR release overlay** — production deploy now builds from checkout via `bearhost-build.yml`; GHCR only when `STREAMPULSE_USE_RELEASE_IMAGES=1` + immutable `IMAGE_TAG`.
3. **Duplicate worker risk** — deploy preflight detects old `streampulse-vps-corpus` containers and exits with stop instructions (does not auto-down).
4. **Migration 000051 missing `generated_at`** — added to CREATE + idempotent `ADD COLUMN IF NOT EXISTS` for materializer SQL parity.

---

## Tests

```
go test ./internal/analytics -count=1  → PASS
```

---

## Go / no-go for cutover phases

| Phase | Status |
|-------|--------|
| Push branch + open PR | **Go** after commits on clean tree |
| BearHost `pg_dump -Fc` backup | **Go** (read-only dump script ready) |
| VPS restore + production deploy | **Hold** until VPS SSH confirmed and old corpus stack stopped |
| Cloudflare tunnel/DNS cutover | **Stop — operator approval required** |
| PR 2 coverage/gap linkage | **Hold** |
| Second corpus worker | **No** |

---

## Next operator steps

1. From WSL: confirm `ssh -i ~/.ssh/id_ed25519 root@23.173.152.156`.
2. On VPS: if old corpus stack running → `docker compose -f deploy/docker-compose.streampulse-vps-corpus.yml down`.
3. Copy `deploy/env/profile-streampulse-vps-production.env.example` → `.local.env` + secrets on VPS.
4. `bash scripts/streampulse-vps-production-backup-bearhost.sh`
5. `STREAMPULSE_PG_RESTORE_CONFIRM=YES_I_HAVE_PROD_BACKUP DUMP_PATH=... bash scripts/streampulse-vps-production-restore.sh`
6. `bash scripts/streampulse-vps-production-deploy.sh`
7. VPS localhost smoke; then **ask** before Cloudflare cutover.
