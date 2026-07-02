# StreamPulse VPS migration unblock audit — 2026-07-02

**Branch:** `fix/vps-migration-hardening` (not pushed)
**Scope:** BearHost backup → VPS pg_restore → production deploy → localhost smoke. **No DNS/tunnel cutover.**

---

## Git / branch

| Item | State |
|------|--------|
| Branch | `fix/vps-migration-hardening` |
| HEAD | `3a88f9f` + repeatability batch (this commit) |
| Unrelated dirty work | `scripts/tmp/` local helpers only — not promoted |
| PR 2 / second worker | **not started** |

---

## Phase execution (2026-07-02, WSL + `~/.ssh/id_ed25519`)

| Step | Result |
|------|--------|
| SSH `root@hosted-production-vps` | **OK** from WSL |
| Old `hosted-production-vps-corpus` stack | **stopped** (`docker compose … down`; no corpus-only containers remain) |
| BearHost backup | **OK** — `runtime/backups/streamclone-bearhost-20260702T004055Z.dump` (~255M) |
| VPS pg_restore | **OK** — project `streamclone-production`, `schema_migrations` **58**, dirty **false** |
| Production deploy | **OK** after ops fixes (see below) |
| Cloudflare cutover | **not done** — awaiting operator approval |

### Deploy fixes (ops session + branch working tree)

1. **Missing S3 env** — VPS `.env` lacked `S3_ENDPOINT` / keys → emote crash-loop (`objstore init failed`). MinIO defaults added on VPS; deploy now runs `streampulse_vps_ensure_emote_storage_env` and documents vars in `profile-hosted-production-vps-production.env.example`.
2. **Duplicate Caddy port merge** — `bearhost-pulse.yml` (`8090:80`) + VPS overlay merged to broken binding. Fixed with `ports: !override` → `127.0.0.1:8090:80`.
3. **Remote compose sourcing** — restore/deploy SSH blocks source `hosted-production-vps-production-compose.sh` on VPS instead of embedding local compose args.
4. **Production compose stack** — helper includes `docker-compose.prod.yml`, `profile-bearhost-prod.env`, `profile-bearhost-pulse.env` (matches successful VPS deploy).

---

## Hosted API (BearHost still serving `api.streampulse.stream`)

| Check | Result |
|-------|--------|
| `GET /v1/extension/health` | **200** ~300–400ms |
| `GET /v1/public/hub?activityWindow=30m` | **200** ~300ms–1s |
| `GET /v1/public/hub?activityWindow=7d` | **200** ~350ms |

Production traffic **still on BearHost** until tunnel cutover.

---

## VPS localhost smoke (`http://127.0.0.1:8090`, post-deploy)

Last read-only verify: **2026-07-02** (WSL SSH audit + `deploy/smoke/bearhost-pulse-api.sh`).

| Check | Result |
|-------|--------|
| `GET /v1/extension/health` | **200** ~3ms |
| `deploy/smoke/bearhost-pulse-api.sh` | **PASS** (`hostedMode=true`, no forbidden keys) |
| `GET /v1/public/hub?activityWindow=30m` | **200** ~0.4s, 24 live channels |
| `GET /v1/public/hub?activityWindow=7d` | **200** ~7ms (cached) |
| `streamclone-pulse-caddy` | **Up**, `127.0.0.1:8090→80` only |
| `streamclone-production-emote-1` | **healthy** |
| `streampulse-analytics-workers` | **1 container, healthy** |
| `streampulse-scraper` | **healthy** (production stack, not old corpus-only) |
| Old corpus-only compose | **no containers** |
| VPS `cloudflared` | **not running** — cutover requires operator tunnel setup |

---

## Postgres on VPS (local SoT, post-restore + ~5m worker)

| Check | BearHost (pre-migrate) | VPS (post-deploy) |
|-------|------------------------|-------------------|
| `schema_migrations` | 58, dirty false | **58, dirty false** |
| `gold_vod_segments` done | 6408 | **6869+** (worker advancing) |
| queued | 1373 | **1375** |
| running | 24 | **0 stale** (active leases OK) |
| failed | — | **24** |
| skipped | 5633 | **5915** |
| stale running (`lease_expires_at < now()`) | **24** | **0** |
| false-done (done gold jobs with unresolved segments) | **0** | **0** |

Stale reclaim **cleared** once worker used local Postgres (no tailnet dependency).

---

## Blockers fixed in branch

**Committed (`3a88f9f` and earlier):**

1. Restore compose project mismatch + `STREAMPULSE_PG_RESTORE_CONFIRM` guard
2. Deploy builds from checkout (`bearhost-build.yml`), not GHCR by default
3. Deploy preflight blocks duplicate corpus workers
4. Migration 000051 `generated_at` column for materializer parity

**Repeatability batch (same commit as compose/deploy hardening):**

5. Remote compose sourcing on VPS (restore/deploy)
6. Full production compose stack in helper (`prod.yml`, bearhost env overlays, Caddy `ports: !override`)
7. Emote S3/MinIO defaults in env example + deploy preflight

---

## Tests

```
go test ./internal/analytics -count=1  → PASS
```

---

## Go / no-go

| Phase | Status |
|-------|--------|
| VPS restore + production deploy | **Go** — localhost smoke passes |
| Cloudflare tunnel/DNS → VPS | **Stop — operator approval required** |
| BearHost decommission | **Hold** — keep as rollback until 24h soak post-cutover |
| PR 2 coverage/gap linkage | **Hold** |
| Second corpus worker | **No** |

---

## Operator approval checklist (before Cloudflare cutover)

1. Confirm VPS localhost checks above still pass (`deploy/smoke/bearhost-pulse-api.sh` with `PULSE_SMOKE_BASE_URL=http://127.0.0.1:8090`).
2. Point Cloudflare tunnel `api.streampulse.stream` → `http://127.0.0.1:8090` on **hosted-production-vps** (not BearHost).
3. Re-run hosted smoke against `https://api.streampulse.stream`.
4. Monitor 24h: hub latency, stale reclaim stays 0, false-done stays 0, single worker healthy.
5. Keep BearHost stack available for rollback until soak passes.

**Do not cut over automatically.**
