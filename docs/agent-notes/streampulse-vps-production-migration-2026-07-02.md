# StreamPulse VPS production migration plan — 2026-07-02

**Goal:** Move production SoT from expiring BearHost (`141.11.243.103`) to bigger VPS
`streampulse-vps` (`23.173.152.156`). API + Postgres + Redis + **single** corpus worker on
one host — no Tailscale dependency on BearHost for DB.

**Status:** Code hardening + migration chain restored locally. **DNS/tunnel cutover not done.**

---

## Phase 0 — Code hardening (this branch)

| Item | Status |
|------|--------|
| `fetchGQLSegmentSerial` durable ledger parity | done |
| Public hub live KPI filter (`gql`/`ivr`/`mixed` excluded) | done |
| Hub cache refresh cadence (30m=30s, 7d=5m) | done |
| Migrations 051–058 on disk | done |
| `go test ./internal/analytics` | pass |

---

## Phase 1 — BearHost backup (read-only)

```bash
bash scripts/streampulse-vps-production-backup-bearhost.sh
```

- Custom-format `pg_dump` (`-Fc`) from `streamclone-postgres-1`.
- Optional: copy latest nightly from `/opt/streamclone/backups/` if fresher.
- Keep BearHost running as rollback until Phase 5 passes.

**Redis:** no backup required for cutover (cache-only).

---

## Phase 2 — Restore on streampulse-vps

```bash
DUMP_PATH=runtime/backups/streamclone-bearhost-*.dump \
  bash scripts/streampulse-vps-production-restore.sh
```

Verify:

```sql
SELECT version, dirty FROM schema_migrations;  -- expect 58, false
SELECT status, count(*) FROM gold_vod_segments GROUP BY 1;
```

---

## Phase 3 — Deploy production stack (local Postgres)

1. Copy `deploy/env/profile-streampulse-vps-production.env.example` →
   `deploy/env/profile-streampulse-vps-production.local.env` on VPS.
2. Fill secrets from BearHost `.env` / `/etc/streamclone/secrets` (Twitch OAuth, scraper key, pulse beta).
3. Ensure `DATABASE_URL` uses `postgres:5432` (not `bearhost` tailnet host).

```bash
bash scripts/streampulse-vps-production-deploy.sh
```

Stack: `postgres`, `redis`, `migrate`, `metadata`, `emote`, `analytics` (API),
`pulse-caddy` (:8090), `scraper`, `analytics-workers` (count=1,
`GOLD_VOD_SEGMENTS_ENABLED=true`, GQL concurrency=2).

**Stop:** decommission `deploy/docker-compose.streampulse-vps-corpus.yml` worker-only stack
after production stack is healthy (avoids duplicate corpus workers).

---

## Phase 4 — Smoke (on VPS localhost, before DNS)

| Check | Target |
|-------|--------|
| `GET /v1/extension/health` | 200 |
| `GET /v1/public/hub?activityWindow=30m` | 200, <10s |
| `GET /v1/public/hub?activityWindow=7d` | 200, <15s |
| Hub top-level keys | no segment/lease leak |
| `gold_vod_segments` stale `running` | drops after reclaimer interval (~3m) |
| False-done gold jobs | 0 |
| `streampulse-analytics-workers` | healthy |
| `deploy/smoke/test-013b-hosted.sh` | pass (against localhost or tunnel preview) |

Segment ledger queries: see `docs/agent-notes/corpus-0b2-hosted-verify.md`.

---

## Phase 5 — Cutover (operator approval required)

1. Point Cloudflare tunnel `api.streampulse.stream` → `http://127.0.0.1:8090` on **streampulse-vps**.
2. Re-run hosted smoke against `https://api.streampulse.stream`.
3. Monitor 24h: hub latency, stale reclaim, false-done, worker health.
4. Keep BearHost stack up (stopped workers OK) for rollback until soak passes.

**Do not** run without explicit approval:

- Cloudflare DNS/tunnel changes
- `docker volume rm` / `make down-clean`
- Second corpus worker
- PR 2 coverage/gap linkage

---

## Rollback

1. Revert Cloudflare tunnel to BearHost `localhost:8090`.
2. `docker compose up` on BearHost if stopped.
3. VPS stack can remain for debugging; production traffic stays on BearHost.

---

## Current hosted baseline (BearHost still live)

Recorded 2026-07-02 — see `corpus-hosted-baseline-2026-07-02.md`:

- Hub 30m/7d: 200, sub-second (post `7197575` / `2660a3a`)
- `schema_migrations`: 58
- Stale `gold_vod_segments.running`: 24 (VPS worker could not reach BearHost PG)
- VPS worker-only deploy: unhealthy (tailnet PG refused)

After VPS SoT migration, stale reclaim should clear locally without tailnet.
