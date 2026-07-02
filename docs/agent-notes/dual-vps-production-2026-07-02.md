# Dual-VPS production topology — 2026-07-02

StreamPulse hosted production uses **two VPS roles**. The laptop is dev/ops only — never production corpus, scraper, or IRC capacity.

## Architecture

```
                    Internet / Cloudflare
                              |
                    api.streampulse.stream
                              |
              +---------------+---------------+
              |   streampulse-vps (SoT)     |
              |   23.173.152.156            |
              +---------------+---------------+
              | Caddy + cloudflared tunnel    |
              | Postgres + Redis (SoT)        |
              | analytics API + emote API     |
              | IRC collector + top-roster    |
              | metadata sampler              |
              | 1× gold corpus canary worker  |
              +---------------+---------------+
                              |
                   Tailscale (private)
                              |
              +---------------+---------------+
              |   BearHost (private workers)  |
              |   141.11.243.103              |
              +---------------+---------------+
              | NO public tunnel              |
              | NO public Grafana/admin/API   |
              | analytics-workers only        |
              | silver + gold drain (scoped)  |
              | optional rollback stack idle  |
              +-------------------------------+

  Laptop (Windows) — Cursor, local :8090 dev stack, probes only
```

## Ownership split

| Host | Public API | Postgres/Redis | IRC admission | Corpus workers | Tunnel |
|------|------------|----------------|---------------|----------------|--------|
| **streampulse-vps** | Yes (`api.streampulse.stream`) | SoT | Yes | Canary (1 gold) + API plane | Yes |
| **BearHost** | No (rollback stack inactive unless DNS flip) | Remote client to VPS SoT | No | Silver + extra gold | **No** |
| **Laptop** | No | No | No | **No** | No |

## BearHost worker start/stop

```bash
# On BearHost after copying env:
cp deploy/env/profile-bearhost-worker.env.example \
   deploy/env/profile-bearhost-worker.local.env
# edit DATABASE_URL, REDIS_URL, secrets paths, Twitch creds

bash scripts/bearhost-worker.sh

docker compose --project-name streamclone-bearhost-worker \
  -f deploy/docker-compose.bearhost-worker.yml ps

docker compose --project-name streamclone-bearhost-worker \
  -f deploy/docker-compose.bearhost-worker.yml down
```

Worker IDs (must stay distinct from VPS canary):

```bash
GQL_WORKER_ID=bearhost-workers
SCRAPER_ID=bearhost-workers
CORPUS_WORKER_ID=bearhost-corpus
```

## Verification probes (read-only)

Public (no auth):

```bash
curl -s https://api.streampulse.stream/v1/extension/health | jq '.status'
curl -s 'https://api.streampulse.stream/v1/public/hub?activityWindow=7d' \
  | jq '{coverage: .coverage, corpus: .corpusPipeline, topEmotes: .topEmotes[0:2]}'
```

Internal corpus must stay closed:

```bash
curl -s -o /dev/null -w '%{http_code}\n' https://api.streampulse.stream/v1/internal/corpus/gaps
curl -s -o /dev/null -w '%{http_code}\n' https://api.streampulse.stream/v1/internal/corpus/readiness
# expect 401 / 404 — never 200 with rows
```

BearHost worker health (local on BearHost only):

```bash
docker compose --project-name streamclone-bearhost-worker \
  -f deploy/docker-compose.bearhost-worker.yml exec analytics-workers /healthcheck
```

## IRC admission restore (operator — after approval)

**Accepted evidence:** 24h top-200 run with ~113 active tracking completed without problems. Do **not** re-run that soak.

Next env target on **streampulse-vps** `analytics` service:

```bash
PULSE_COLLECTOR_ENABLED=true
PULSE_TOP500_ADMISSION_ENABLED=true
PULSE_TOP500_ADMISSION_TOP_N=200
PULSE_MAX_ACTIVE_CHANNELS=200
MAX_CONCURRENT_TRACKED_CHANNELS=200
TIER0_ENABLED=true
```

(`PULSE_TOP_ROSTER_ADMISSION_*` is still accepted as a legacy alias in Go config.)

Verify via public hub only:

- `coverage.collectorTracking` rises toward live demand (not capped at 10)
- `coverage.collectorActive <= 200`
- chat/emote rollups appear for admitted channels
- coverage moves from `critical` toward `degraded`/`healthy` as live rooms fill in

Rollback:

```bash
PULSE_TOP_ROSTER_ADMISSION_ENABLED=false
```

Use `PULSE_TOP500_ADMISSION_ENABLED=false` (canonical) or the legacy `PULSE_TOP_ROSTER_*` alias.

### Path toward top-500 (after top-200 is stable in normal operation)

```bash
PULSE_TOP500_ADMISSION_TOP_N=500
PULSE_MAX_ACTIVE_CHANNELS=300
MAX_CONCURRENT_TRACKED_CHANNELS=300
```

Raise active cap toward 500 only if CPU/RAM/network/IRC reconnect metrics stay healthy. This is deliberate scaling — not re-proving top-200.

## Corpus burn-down (aggregate only)

Use public hub `corpusPipeline` / `corpus` fields first. Admin routes require token — never paste raw rows.

| Plane | VPS role | BearHost role |
|-------|----------|---------------|
| Silver | Auto-enqueue canary | Primary drain (`BACKFILL_SILVER_WORKER_COUNT=2`) |
| Gold | 1 canary worker | Extra worker with conservative GQL (`ANALYTICS_VOD_GQL_CONCURRENCY=1`) |
| Archive/R2 | SoT credentials on VPS | Worker-scoped R2 read/write for mirror/verify jobs |

**Bottleneck classes (aggregate):** gold queue age, low running count, Twitch GQL/proxy rate limits, VOD availability — not CPU alone. Prefer BearHost for retry-safe silver/gold-lite streams with existing rollups; do not blast full GQL across top-500.

## Rollback procedure

1. **IRC:** `PULSE_TOP500_ADMISSION_ENABLED=false` on streampulse-vps; restart analytics.
2. **Workers:** `docker compose ... down` on BearHost worker project only.
3. **Public API:** BearHost rollback stack + DNS/tunnel flip is **operator-only** and **not** default — streampulse-vps remains canonical unless explicitly approved.

## Related files

- `deploy/docker-compose.bearhost-worker.yml`
- `deploy/env/profile-bearhost-worker.env.example`
- `deploy/docker-compose.streampulse-vps-production.yml`
- `docs/agent-notes/streampulse-vps-production-migration-2026-07-02.md`
- `docs/bearhost-production.md` (rollback host runbook)
