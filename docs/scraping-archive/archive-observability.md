# Archive observability — operator runbook

Status: **draft**
Spec: [corpus-requirements.md § Part II](corpus-requirements.md#part-ii--job-progress--observability)
Related: [bearhost-production.md](../bearhost-production.md) · [azure-archive-setup.md](../azure-archive-setup.md)

---

## Purpose

This runbook covers **how operators monitor and control** the Streamclone global archive corpus on BearHost:

- **Postgres** — exact job progress (`archive_jobs`, `archive_job_items`)
- **Admin UI** — `/admin/archive`, `/admin/jobs`, `/admin/coverage`
- **Prometheus + Grafana** — trends, queue depth, failure rates (optional profile)

Grafana is **not** the source of truth for progress. Use the admin UI or CLI when you need exact counts and per-item errors.

---

## Source of truth

| Question | Where to look |
|----------|---------------|
| Is Bronze running? | Admin UI active jobs, or `go run ./cmd/backfill jobs list --status=running` |
| How far along? | `completed_items / total_items` on job row |
| What failed? | `archive_job_items` where `status=failed`, or admin Failure table |
| What's the blob URI? | Item `output_uri` or `archive_exports.gcs_uri` |
| Historical trend? | Grafana archive dashboard |
| Is worker dead? | `heartbeat_at` stale → job `status=stale` |

---

## BearHost defaults

| Setting | BearHost 8 GB | Notes |
|---------|---------------|-------|
| `observability` compose profile | **Off** | Saves RAM; enable when monitoring needed |
| `pulse` profile | Off | Legacy Pulse analytics stack; use `observability` for archive |
| `ARCHIVE_JOB_PROGRESS_ENABLED` | `true` | When Part II shipped |
| `ADMIN_ARCHIVE_REQUIRE_TOKEN` | `true` | Same token family as setup-control |
| `/metrics` | Internal Docker only | Never add Caddy route |

---

## Enabling observability on BearHost

### 1. Merge observability overlay (when shipped)

```bash
docker compose \
  --env-file .env \
  --env-file deploy/env/profile-full.env \
  --env-file deploy/env/profile-archive.env \
  --env-file deploy/env/profile-bearhost-prod.env \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.prod.yml \
  -f deploy/docker-compose.bearhost-prod.yml \
  -f deploy/docker-compose.observability.yml \
  --profile observability up -d
```

### 2. Access Grafana (SSH tunnel)

On your PC:

```bash
ssh -i ~/.ssh/id_ed25519_bearhost_streamclone -L 3000:127.0.0.1:3000 streamclone@141.11.243.103
```

Open `http://localhost:3000` — default credentials from compose env (change on first login).

### 3. Prometheus

Internal only (`prometheus:9090` on Docker network). Grafana datasource provisioned at `deploy/grafana/provisioning/datasources/prometheus.yml`.

---

## Admin UI access

1. Ensure setup-control token is available locally (same path as PulseWire operator tools).
2. Navigate to `http://141.11.243.103/admin/archive` (or local `http://localhost:8090/admin/archive`).
3. Token is sent as `X-Setup-Control-Token` on admin API calls.

**Do not** expose `/v1/admin/*` without `ADMIN_ARCHIVE_REQUIRE_TOKEN=true`.

---

## CLI quick reference

```bash
# List running / recent jobs
docker compose exec analytics go run ./cmd/backfill jobs list

# Job detail
docker compose exec analytics go run ./cmd/backfill jobs show --job-id=<uuid>

# Retry failed items only
docker compose exec analytics go run ./cmd/backfill jobs retry-failed --job-id=<uuid>

# Resume stale job (skips succeeded items)
docker compose exec analytics go run ./cmd/backfill jobs resume --job-id=<uuid>

# Coverage
docker compose exec analytics go run ./cmd/backfill coverage report
docker compose exec analytics go run ./cmd/backfill coverage verify-blobs
docker compose exec analytics go run ./cmd/backfill coverage stale --older-than 7d

# Trigger jobs
docker compose exec analytics go run ./cmd/backfill bronze run-once --top-n 200
docker compose exec analytics go run ./cmd/backfill emotes snapshot --top-n 200
docker compose exec analytics go run ./cmd/backfill silver enqueue --top-n 200 --days 90
```

---

## Stale jobs and resume

**Stale detection:** If `status=running` and `heartbeat_at` is older than `ARCHIVE_JOB_STALE_AFTER` (default 10 minutes), a background task sets `status=stale`.

**Typical causes:**

- Worker container restarted mid-job
- Long Camoufox TT scrape without heartbeat (tune interval or disable stale for silver if needed)
- VPS OOM kill

**Resume behavior:**

1. `resume --job-id` sets job to `running`, refreshes heartbeat.
2. Items with `status=succeeded` or `skipped` are **not** reprocessed.
3. Items with `status=failed`, `queued`, or stale `running` are re-queued.
4. `retry-failed` only touches `failed` items and increments `attempts`.

Document max attempts in env (`ARCHIVE_JOB_MAX_ATTEMPTS`, default 3) when implemented.

---

## Security checklist

- [ ] `/metrics` not in Caddyfile public routes
- [ ] Grafana bound to `127.0.0.1` or protected subpath with auth
- [ ] Admin routes require setup-control / operator token
- [ ] Scraper has no host port publish
- [ ] Postgres / Redis not published to host
- [ ] Audit events written for retry / resume / cancel / enqueue

See [security.md](../security.md) for secret handling.

---

## Grafana dashboard

Primary dashboard (planned): `deploy/grafana/dashboards/streamclone-archive.json`

Existing ops dashboard: `deploy/grafana/dashboards/streamclone-ops.json` (Pulse pipeline + some archive metrics after Part II).

**Key panels to watch during backfill:**

| Panel | Alert if |
|-------|----------|
| Running jobs | Stuck &gt; 24h without progress |
| Queue depth | Monotonic increase over 6h |
| Scraper failure rate | &gt; 20% over 10m |
| Azure upload failures | Any sustained spike |
| Stale channels | Above roster size × 10% |
| Worker heartbeat age | &gt; 10m |

---

## Prometheus alerts

Rules file (planned): `deploy/prometheus/alerts/archive.yml`

Route alerts to email/Discord via Alertmanager when configured. Minimum viable: check Grafana alert rules on BearHost manually during first corpus backfill.

---

## Cron suggestions (VPS)

```bash
# /etc/cron.d/streamclone-archive-ops
# Daily coverage snapshot + blob verify (off-peak UTC)
30 5 * * * streamclone cd /opt/streamclone/app && docker compose exec -T analytics go run ./cmd/backfill coverage report --out=/opt/streamclone/backups/coverage-$(date +\%F).json
0 6 * * * streamclone cd /opt/streamclone/app && docker compose exec -T analytics go run ./cmd/backfill coverage verify-blobs

# Stale job report (no auto-resume unless operator opts in)
0 */4 * * * streamclone cd /opt/streamclone/app && docker compose exec -T analytics go run ./cmd/backfill jobs list --status=stale
```

---

## Troubleshooting

| Symptom | Check | Action |
|---------|-------|--------|
| Job stuck at 0% | `total_items` set? | Worker bug or empty roster — check Tier-0 |
| All silver items fail | Scraper logs | Residential proxy; reduce concurrency |
| Heartbeat stale but worker alive | Long TT scrape | Increase `ARCHIVE_JOB_STALE_AFTER` temporarily |
| Admin UI 401 | Token | Refresh setup-control token |
| Grafana empty | Scrape targets | Confirm analytics on Docker network; check Prometheus targets |
| Metrics on public URL | Caddy | Remove route; verify with `curl http://141.11.243.103/metrics` → must 404 |

---

## Rollback

1. Set `ARCHIVE_JOB_PROGRESS_ENABLED=false` — workers revert to legacy logging only.
2. Set `ADMIN_ARCHIVE_ENABLED=false` — disable admin API/UI.
3. Stop observability profile — `docker compose --profile observability down`.
4. Postgres job tables remain; safe to leave inert.

Full spec rollback: [corpus-requirements.md](corpus-requirements.md).

---

## Document history

| Date | Change |
|------|--------|
| 2026-06-20 | Initial operator runbook for Part II observability |
