# Streamclone — quick links

Production host: **http://141.11.243.103/** (BearHost VPS, HTTP until domain + HTTPS).

## Site

| What | URL |
|------|-----|
| Home / directory | http://141.11.243.103/ |
| Analytics | http://141.11.243.103/analytics |
| Admin archive (operator) | http://141.11.243.103/admin/archive |
| Local dev | http://localhost:8090 |
| GitHub | https://github.com/Aron-Chu/streamclone |

## Login

| Where | Works? |
|-------|--------|
| **localhost:8090** | Yes — “Sign in with Twitch” uses loopback-only dev/device auth |
| **141.11.243.103** | **No** — same UI, but token import is blocked off localhost by design |

Browse analytics without login. Public Twitch OAuth for the VPS needs HTTPS + domain (see [multi-user/requirements.md](multi-user/requirements.md)).

## Pulse Wire

| Where | Status |
|-------|--------|
| **BearHost** | **Off** — slim prod profile (`VITE_PULSE_WIRE_ENABLED=false`, no `storygraph` service) |
| **Local full stack** | Enable with `pulse-wire` compose profile + `PULSE_WIRE_ENABLED=true` → http://localhost:8090/pulse-wire |

Not an old deployment; Pulse Wire was never part of the BearHost cut.

## BearHost make targets (from your PC)

Run `make bearhost-help` for the full list. Common ops:

| Task | Command |
|------|---------|
| Disable local scrape (VPS owns corpus) | `make local-vps-only` |
| **VPS corpus-only** (stop playback/UI) | `make bearhost-corpus-only` |
| Bronze / VOD job summary | `make bearhost-bronze-status` |
| First-time Grafana on VPS | `make grafana-setup` |
| SSH tunnel → VPS Grafana | `make grafana-up` → **http://localhost:3001** |
| Keep tunnel alive (Windows) | `make grafana-watch-install` (Task Scheduler) or `make grafana-watch-install-cron` (WSL cron) |
| One tunnel health check | `make grafana-watch` |
| Stop tunnel watchdog | `make grafana-watch-uninstall` |
| Stop Grafana tunnel | `make grafana-stop` |
| Push dashboard edits to VPS | `make grafana-sync` |
| Check Prometheus/Grafana on VPS | `make bearhost-observability-status` |
| Rsync repo to VPS | `make bearhost-rsync` |

On Windows, `make` uses PowerShell + WSL for SSH/rsync. On Linux/macOS, the same targets call the bash scripts directly.

## VOD / bronze collection status (BearHost VPS only)

Local dev should **not** scrape — run `make local-vps-only` once to stop the local scraper and disable Tier-0/Bronze/backfill workers. Corpus runs on the VPS.

Workers must have **`CORPUS_WORKERS_ENABLED=1`** on the VPS after corpus preflight ([bearhost-production.md](bearhost-production.md)).

**From your PC:**

```bash
make bearhost-bronze-status
```

**On the VPS directly** (after `ssh streamclone@141.11.243.103`):

```bash
cd /opt/streamclone/app
bash scripts/bearhost-bronze-status.sh
```

**Local dev only** (different machine / your docker stack — not production data):

```powershell
go run ./cmd/backfill bronze status
.\scripts\bronze-acceptance-run.ps1 -Stage smoke -DurationMinutes 5 -PollMinutes 1
```

Roster size: up to **500** channels (`BRONZE_TOP_N`), **200** concurrently tracked (`MAX_CONCURRENT_TRACKED_CHANNELS` in [profile-bearhost-prod.env](../deploy/env/profile-bearhost-prod.env)).

## Grafana (VPS archive + scraper health)

Optional on BearHost (~200 MB RAM). **Not a full Grafana revamp** — `observability` profile + **Streamclone Archive** dashboard only.

**First time** (rsync, rebuild workers metrics, start Prometheus + Grafana):

```bash
make grafana-setup
```

**View locally** (SSH tunnel in background):

```bash
make grafana-up
```

Open **http://localhost:3001/d/streamclone-archive/streamclone-archive** (login `admin` / `streampulse`).

**Important:** local Pulse Grafana also uses **http://localhost:3000** (InfluxDB). That instance does **not** have VPS archive metrics — use **:3001** after the tunnel, not :3000.

If **everything** on `:3001` is blank, the SSH tunnel probably stopped — run `make grafana-up` again, or install the watchdog: `make grafana-watch-install` (checks every 5 minutes and restarts the tunnel). Log: `~/.streamclone/grafana-tunnel-watch.log` in WSL.

### Which dashboards work where?

VPS observability Grafana (`:3001` via tunnel) has **Prometheus only** (`prometheus-obs`). It does **not** have InfluxDB, storygraph, or local-dev-only metrics.

| Dashboard | URL | Works on `:3001` (VPS tunnel)? | Why |
|-----------|-----|----------------------------------|-----|
| **Streamclone Archive** | `/d/streamclone-archive/...` | **Partial** | Bronze index, coverage, workers `up` should show data. Jobs running/queued, backfill queue, scraper rate stay empty until those metrics exist / workers run. |
| **Streamclone Ops** | `/d/streamclone-ops/...` | **Mostly no** | Built for local full stack — Pulse timeseries, HLS, chat queue, scraper charts. VPS Prometheus only scrapes `analytics` + `analytics-workers`. |
| **Pulse Wire** | `/d/streamclone-pulse-wire/...` | **No** | Needs `storygraph` metrics. Pulse Wire is **off** on BearHost by design. |
| **Emote Pulse** | `/d/streamclone-emote-pulse/...` | **No** | All panels use **InfluxDB**. Use local Pulse Grafana on **`:3000`** (`pulse` compose profile), not the VPS tunnel. |

Audit what's actually queryable: `bash scripts/bearhost-grafana-dashboard-audit.sh` (with tunnel up).

**After editing dashboards** in git:

```bash
make grafana-sync
make grafana-up
```

Panels: archive jobs, backfill queue, coverage by tier, bronze index vs target, upload rate, plus **Scraper & workers (BearHost VPS)** — workers `up`, TT error rate, success/min, active syncs, GQL throttles.

Datasource: unified **`Prometheus` (`uid: prometheus`)** — same dashboard JSON locally (Pulse) and on VPS (tunnel to `prometheus-obs`).

## Ops runbooks

- [bearhost-production.md](bearhost-production.md) — deploy, smoke, corpus, cron
- [scraping-archive/archive-observability.md](scraping-archive/archive-observability.md) — jobs, Grafana, restore drills
- [benchmarks/bronze-acceptance-smoke-operator.md](benchmarks/bronze-acceptance-smoke-operator.md) — 100/200 channel acceptance runs
