# BearHost production runbook



Streamclone full production on legacy-rollback-host **legacy-rollback-host** — IP-only HTTP until a domain and ACME are configured.



**Supersedes:** Azure Terraform hybrid rollout for this host. Do not apply `deploy/terraform/azure/compute` for BearHost.



---



## Architecture



| Public | Internal (Docker network) |

|--------|---------------------------|

| Caddy `:80` / `:443` | frontend, metadata, chat, video, emote, analytics, **analytics-workers**, scraper, postgres, redis, minio, mediamtx, migrate |



- **UFW:** 22, 80, 443 only

- **Scraper:** no host port publish; smoke uses `docker compose exec scraper curl …`

- **Secrets:** `/etc/streamclone/secrets/` (mode 700), mounted read-only into workers



### Corpus plane (locked — TASK-002/003)



| Service | Role | Corpus flags |

|---------|------|--------------|

| `analytics` | HTTP API (streams, sync status, future admin routes) | **Always off** on BearHost — `ARCHIVE_ENABLED`, `BRONZE_ENABLED`, `BACKFILL_ENABLED`, `GOLD_BACKFILL_ENABLED`, `TIER0_ENABLED`, `ARCHIVE_PG_DUMP_NIGHTLY` all `false` |

| `analytics-workers` | Long-running corpus plane (bronze, backfill, emote export, tier-0) | **On only after preflight** — gated by `CORPUS_WORKERS_ENABLED` (default `false` in `profile-bearhost-prod.env`) |



**Preflight** (`scripts/bearhost-corpus-preflight.sh`, gate 0 in `bearhost-smoke.sh`):



1. Host file `${STREAMCLONE_SECRETS_DIR}/azure-archive-connection-string` exists (maps to container `ARCHIVE_AZURE_CONNECTION_STRING_FILE`).

2. `.env` contains a complete Twitch credential pair: `TWITCH_CLIENT_ID` + `TWITCH_CLIENT_SECRET` or `TWITCH_OAUTH_CLIENT_ID` + `TWITCH_OAUTH_CLIENT_SECRET`.

Quick host check before enabling corpus:

```bash
bash scripts/bearhost-vps-env-check.sh
BEARHOST_CORPUS_PREFLIGHT_ONLY=1 bash scripts/bearhost-smoke.sh
```



**Enable corpus after preflight:**



```bash

# On VPS — install secret first (see Azure archive below)

bash scripts/bearhost-smoke.sh   # gate 0 must pass

# Edit deploy/env/profile-bearhost-prod.env (or VPS .env):

CORPUS_WORKERS_ENABLED=1

bash scripts/bearhost-deploy-phased.sh   # phase 4 picks up flag; or:

docker compose ... up -d --force-recreate analytics-workers

```

Corpus smoke and restore drill can run without host Go:

```bash
BEARHOST_USE_DOCKER_GO=1 bash scripts/bearhost-corpus-smoke.sh
STREAM_ID=<id> BEARHOST_USE_DOCKER_GO=1 bash scripts/archive-restore-drill.sh
```



**Fail closed:** Without secret or Twitch creds, `bearhost-deploy-phased.sh` exports `CORPUS_WORKERS_ENABLED=0` so workers stay healthy without Azure init crash loops. `analytics` never runs corpus workers regardless.

### IVR shadow canary (PROD_SHADOW_CANARY_ONLY)

Non-mutating IVR comparison runs **only on the corpus workers plane**, not on Pulse API mode (`profile-bearhost-pulse.env` keeps `GOLD_BACKFILL_ENABLED=false`).

Merge overlay after corpus profile:

```bash
# deploy/env/profile-bearhost-corpus-ivr-shadow.env
GOLD_IVR_ENABLED=true
GOLD_IVR_SHADOW_MODE=true
GOLD_IVR_LITE_ENABLED=false
GOLD_IVR_PEAKS_ONLY_ENABLED=false
GOLD_IVR_CANONICAL_REPLACE=false
GOLD_IVR_ENABLED_CHANNEL_ALLOWLIST=ludwig
```

Requires migration **000050** (`chat_source`, `source_confidence`, `chat_source_detail` on `analytics_minute_rollups`). Startup logs `gold_ivr effective config` from analytics and backfill.

**HOLD:** Do not merge the IVR shadow overlay or recreate `analytics-workers` until migration 000050 passes preflight (`source_columns=3`).

Read-only preflight (from dev machine):

```bash
bash scripts/bearhost-migration-000050-preflight.sh
```

Local proof: `bash scripts/bench/ivr-shadow-reconcile-proof.sh`. This is **not** IVR prod — artifacts only; GQL remains canonical.

#### IVR shadow canary verification (read-only after deploy)

Run on BearHost after corpus workers are up with `profile-bearhost-corpus-ivr-shadow.env` merged:

```bash
# 1) Corpus worker mode active
docker compose ps | grep -E 'analytics-workers|analytics-1'

# 2) Startup config (must match safe block)
docker compose logs analytics-workers 2>&1 | grep -i 'gold_ivr effective' | tail -3
# Expect: enabled=true shadow=true lite=false peaks_only=false canonical_replace=false allowlist=[ludwig]

# 3) Shadow artifacts (no raw chat bodies)
docker compose exec analytics-workers ls -la runtime/ivr-shadow/ 2>/dev/null | tail -10
# JSON must include: shadow_only=true wrote_rollups=false updated_stream_metadata=false

# 4) DB leakage check (requires migration 000050)
docker exec streamclone-postgres-1 psql -U app -d streamclone -P pager=off -c "
SELECT chat_source, source_confidence, COUNT(*)
FROM analytics_minute_rollups
WHERE chat_source='ivr'
GROUP BY 1,2;"
# Expect: zero rows
```

Switch back to Pulse API mode when done validating; BearHost runs one heavy mode at a time on 8 GB.

### Hosted API boundary (Layer-2 timelines)

When `PULSE_HOSTED_MODE=true`, unauthenticated clients must not read full minute timelines from legacy desktop routes. Gated paths:

- `GET /v1/analytics/channels/{login}/live` — requires beta key or device token (including `?sparse=false`)
- `GET /v1/analytics/streams/{streamID}` — requires beta key or device token
- `GET /v1/analytics/streams/{streamID}/replay-heatmap` — same
- `GET /v1/portal/analytics/streams/{streamID}/minutes` — same (portal sanitization)

Public aggregate routes remain unauthenticated: `/v1/public/hub`, `/v1/public/emotes/overview` (after binary deploy).

### Public emotes overview deploy

Route: `GET /v1/public/emotes/overview?range=7d` registered in `internal/analytics/public_api.go`. A **404** on prod means the deployed analytics image predates the route — not a Caddy issue. After deploy, expect `200` with `state` in `ready|degraded|empty|unavailable` and `aggregateOnly:true`. Never raw chat or storage credentials.

Migrations **000051/000052** (public emote materialization tables) are **optional** for this batch — the predeploy gate emits `MIGRATION_PUBLIC_EMOTES=WARN` when missing. Do not claim “Full Global Emotes ready” until those migrations are applied, the materializer has run, and smoke returns `200` with real aggregate data.

### Mandatory Pulse analytics deploy order

Script-enforced gates replace ad-hoc “migrate then pray” steps. **Do not** `force-recreate analytics` until migration **000050** passes the predeploy gate.

1. **Pre-push review:** `git show --name-status 0b02ef6` — confirm Commit A scope before push.
2. Push Commit A + Commit B (hosted auth boundary + gate scripts).
3. `make bearhost-rsync`
4. **`BEARHOST_ANALYTICS_GATE_REMOTE=1 make bearhost-analytics-predeploy-gate`** — **stop** if `BLOCK_ANALYTICS_RECREATE=1` (never bare gate from dev when local Docker is up).
5. On VPS: `make migrate` (000045–000050 required; 000051/000052 optional this batch).

**Migration chain parity:** Migrations **000045–000049** must remain in the repo whenever BearHost prod schema ≥ 45. `bearhost-rsync --delete` must not drop migration files that prod has already applied — otherwise `golang-migrate` fails at version 49 with “no migration found for version 49”.

6. Re-run **`BEARHOST_ANALYTICS_GATE_REMOTE=1 make bearhost-analytics-predeploy-gate`** — require `ANALYTICS_DEPLOY_GATE=PASS` and `BLOCK_ANALYTICS_RECREATE=0`.
7. `bash scripts/bearhost-pulse-api.sh` — gate runs automatically before analytics recreate (`BEARHOST_ANALYTICS_GATE_LOCAL=1` inside script; break-glass: `BEARHOST_SKIP_ANALYTICS_DEPLOY_GATE=1`).
8. `bash scripts/pulse-hosted-boundary-smoke.sh` — require `PUBLIC_BOUNDARY=PASS`.
9. Optional: `PULSE_BETA_KEY=... bash scripts/pulse-hosted-boundary-smoke.sh` for `CHART_CANARY` + `VOD_EXTENSION_CANARY`.

**Do-not box:** IVR shadow overlay (`profile-bearhost-corpus-ivr-shadow.env`) remains **HOLD** until migration 000050 + analytics deploy + corpus workers + Ludwig artifacts + zero `chat_source=ivr` rows.

Legacy scripts that grep `MIGRATION_000050=` should call `scripts/bearhost-migration-000050-preflight.sh` (thin wrapper around the full gate).

---



## Recommended workflow: local checkout → VPS build



GHCR `latest` tags can lag behind your working tree. Default BearHost profile builds **from the rsynced repo** (no `docker login` for app services).



### 1. Sync local → VPS



From WSL (repo root):



```bash

make bearhost-rsync

# or: bash scripts/bearhost-rsync-to-vps.sh

```



From Windows PowerShell:



```powershell

powershell -ExecutionPolicy Bypass -File scripts/bearhost-rsync-to-vps.ps1

```



| Path | Remote |

|------|--------|

| Streamclone checkout | `/opt/streamclone/app` |

| Sibling `../streamclone-scraper` (if present) | `/opt/streamclone/streamclone-scraper` |



Excludes: `.git`, `node_modules`, `frontend/node_modules`, `.env`, `.env.local`, `runtime`, `pg-data`.



SSH key: `~/.ssh/legacy-rollback-key` → `streamclone@legacy-rollback-host`.



### 2. Validate compose merge locally



```bash

make bearhost-config-check        # GHCR release path + build-local path

make bearhost-config-check-local  # build-local only (no release.yml)

```



Build-local merge (on VPS or WSL):



```bash

docker compose \

  --env-file .env \

  --env-file deploy/env/profile-full.env \

  --env-file deploy/env/profile-archive.env \

  --env-file deploy/env/profile-bearhost-prod.env \

  -f deploy/docker-compose.yml \

  -f deploy/docker-compose.prod.yml \

  -f deploy/docker-compose.bearhost-prod.yml \

  -f deploy/docker-compose.bearhost-build.yml \

  --profile scraper config

```



`profile-bearhost-prod.env` sets `BEARHOST_BUILD_LOCAL=1` and `SCRAPER_USE_IMAGES=0`.



### 3. Phased deploy on VPS



```bash

ssh -i ~/.ssh/legacy-rollback-key streamclone@legacy-rollback-host

cd /opt/streamclone/app

bash scripts/bearhost-deploy-phased.sh

BEARHOST_SKIP_SYNC=1 bash scripts/bearhost-smoke.sh   # first boot on empty DB

bash scripts/bearhost-smoke.sh

```



`bearhost-deploy-phased.sh` runs `docker compose build` + `up -d` when `BEARHOST_BUILD_LOCAL=1`. Caddy stays `caddy:2` from Docker Hub.



---



## GHCR release path (rollback / optional)



Set `BEARHOST_BUILD_LOCAL=0` and `SCRAPER_USE_IMAGES=1` in `.env` on VPS, then use release merge:



```bash

docker compose \

  ... \

  -f deploy/docker-compose.release.yml \

  -f deploy/docker-compose.prod.yml \

  -f deploy/docker-compose.bearhost-prod.yml \

  --profile scraper pull

```



Requires `docker login ghcr.io` if images are private.



### Optional: push fresh images from local



Build and tag with git SHA, push to GHCR, set `IMAGE_TAG` on VPS — useful when you want registry-based deploy without rsync:



```bash

SHA=$(git rev-parse --short HEAD)

docker compose -f deploy/docker-compose.yml build metadata video chat analytics emote frontend

# tag + push to ghcr.io/aron-chu/streamclone/<service>:${SHA}

```



---



## Bootstrap (first time)



```bash

ssh -i ~/.ssh/legacy-rollback-key root@legacy-rollback-host

curl -fsSL https://raw.githubusercontent.com/Aron-Chu/streamclone/master/scripts/bearhost-bootstrap.sh | bash

# re-login as streamclone

ssh -i ~/.ssh/legacy-rollback-key streamclone@legacy-rollback-host

```



Layout:



```

/opt/streamclone/app/                 # rsync target (repo root)

/opt/streamclone/streamclone-scraper/ # scraper sibling (build context)

/opt/streamclone/backups/             # pg_dump output

/etc/streamclone/secrets/             # azure connection string, etc.

```



---



## `.env` minimum (never commit)



Copy from local `.env` / `.env.local` manually on VPS (rsync excludes secrets):



- Twitch OAuth client + secret, Helix tokens as needed

- `SCRAPER_API_KEY`

- Optional Flame proxy vars



VPS baseline:



```

APP_DOMAIN=legacy-rollback-host

PUBLIC_ORIGIN=http://legacy-rollback-host

ACME_EMAIL=placeholder@example.com

BEARHOST_BUILD_LOCAL=1

SCRAPER_USE_IMAGES=0

SCRAPER_EPHEMERAL_BROWSER=false

STREAMCLONE_SECRETS_DIR=/etc/streamclone/secrets

```



Azure archive (optional):



```bash

sudo install -m 600 -o streamclone -g streamclone \

  ~/azure-archive-connection-string \

  /etc/streamclone/secrets/azure-archive-connection-string

bash scripts/bearhost-vps-env-check.sh

```



---



## Smoke gates



`scripts/bearhost-smoke.sh` runs **nine** checks (compose merge matches deploy mode via `BEARHOST_BUILD_LOCAL`):



0. Corpus preflight — Azure secret file on host + Twitch client creds in `.env` (non-fatal when `CORPUS_WORKERS_ENABLED=false`)

1. `docker compose ps` — no unhealthy / crash-loop

2. Scraper `/health` (via exec)

3. Scraper POST `/v2/scrape` (one TwitchTracker page)

4. `http://legacy-rollback-host/` frontend 200

5. Metadata + analytics via Caddy 200

6. Trigger sync → Redis `analytics:sync:*` key

7. Postgres `analytics_minute_rollups` count increases

8. No OOM / restart storm in recent logs



Skip sync on empty DB: `BEARHOST_SKIP_SYNC=1 bash scripts/bearhost-smoke.sh`



---



## Nightly backup and cron automation

Install all recommended production cron jobs (backup, coverage, quarterly restore drill):

```bash
bash scripts/bearhost-cron-install.sh
crontab -l | grep streamclone-bearhost-ops
```

Or add backup only:

```bash
# crontab -u streamclone -e
0 3 * * * /opt/streamclone/app/scripts/bearhost-pg-backup.sh
```

Dumps: `/opt/streamclone/backups/streamclone-*.sql.gz` (14-day retention).

Coverage snapshots: `/opt/streamclone/backups/coverage/`. Restore drill logs: `/opt/streamclone/backups/cron/`.

---

## Optional observability (Prometheus + Grafana)

Off by default on the 8 GB VPS. When you need trend charts during a corpus backfill:

```bash
bash scripts/bearhost-observability.sh up
# on your PC:
ssh -i ~/.ssh/legacy-rollback-key -L 3001:127.0.0.1:3000 streamclone@legacy-rollback-host
# open http://localhost:3001/d/streamclone-corpus-global/streamclone-corpus-global
bash scripts/bearhost-observability.sh down   # reclaim RAM when done
```

See [archive-observability.md](scraping-archive/archive-observability.md).

---

## Logins and multiple users

Streamclone today is a **single shared corpus** with **per-browser Twitch sessions**, not multi-tenant SaaS.

| Layer | How it works |
|-------|----------------|
| **Viewer login** | Twitch OAuth (or dev token import) via the **chat** service. Redis stores `auth:session:<id>`; browser cookie `streamclone_session` (30-day TTL). Each browser/profile gets its own session ID — many users can be logged in at once with different Twitch accounts. |
| **Analytics reads** | Mostly **anonymous/public** through Caddy; no session required for channel charts and rollups. |
| **Archive admin** | Separate **`ADMIN_ARCHIVE_TOKEN`** header (`X-Admin-Archive-Token`) — not tied to Twitch login; CLI-only on public HTTP BearHost. |
| **PulseWire / setup** | Optional; uses setup-control token when enabled — disabled on current BearHost profile. |

Logging in as user A does **not** isolate data: Postgres rollups and archive blobs are global. Follows and `/v1/me` are scoped to the Twitch user behind the session cookie.

Planned multi-user personalization (saved views, job dedupe, API keys) is documented in [multi-user/requirements.md](multi-user/requirements.md) — **not implemented yet**.

### Why “Sign in with Twitch” fails on the VPS

The UI only implements **loopback dev auth** today (`/v1/auth/dev/*`), not a public OAuth redirect. The chat handler allows token import only when the request host/origin is **localhost** (`allowDevTokenImport` in `internal/chat/auth/handler.go`). On `legacy-rollback-host`, `/v1/me` returns `canImportLocalToken: false` — the frontend shows *“Local token auth is only available on localhost through the local proxy.”*

**Workarounds now:** browse analytics anonymously on the VPS; sign in on **localhost:8090** for follows/VOD relay features.

**Production login later:** register Twitch OAuth redirect for your HTTPS domain, set `TWITCH_DEV_TOKEN_IMPORT_ENABLED=false`, and ship public OAuth UI (tracked in multi-user requirements).

### Pulse Wire on BearHost

**Intentionally off** — not a stale deploy. BearHost uses a slim profile: `VITE_PULSE_WIRE_ENABLED=false`, no `storygraph` / `pulse-wire` compose services, and `deploy/nginx.bearhost.conf` has no Pulse Wire upstream. Enable locally with the `pulse-wire` profile if you need `/pulse-wire`.

---



## HTTPS cutover (when domain exists)



1. DNS A record → `legacy-rollback-host`

2. Set `APP_DOMAIN=your.domain`, real `ACME_EMAIL` in `.env`

3. In `deploy/docker-compose.bearhost-prod.yml`, swap Caddy mount to `deploy/Caddyfile` (or remove bearhost Caddy override)

4. Update `PUBLIC_ORIGIN` and profile URLs to `https://`

5. Re-run phased deploy or `$COMPOSE up -d --force-recreate caddy frontend`

6. Update Twitch OAuth redirect URIs



---



## Cutover from local PC



When all smoke gates pass:



1. BearHost is **active** production

2. Stop local Docker stack (`make stop`) — no local workers/scraper/postgres

3. Keep local checkout + `.env` for **48h rollback**

4. Rollback: `make up` on PC if VPS fails



---



## Troubleshooting



| Symptom | Action |

|---------|--------|

| Scraper build fails | Ensure `/opt/streamclone/streamclone-scraper` exists (`make bearhost-rsync` from machine with sibling checkout) |

| GHCR 401 (release mode) | `docker login ghcr.io` on VPS |

| Workers exit on archive init | Corpus off by default (`CORPUS_WORKERS_ENABLED=false`); run `bash scripts/bearhost-corpus-preflight.sh` (or smoke gate 0), install Azure secret, set `CORPUS_WORKERS_ENABLED=1`, recreate `analytics-workers` |
| Frontend nginx `storygraph` error | Build-local uses `deploy/nginx.bearhost.conf` (no Pulse Wire upstream); ensure bearhost-prod overlay is merged |
| Windows-edited shell scripts fail in WSL | Re-save scripts with LF (`wsl python3 -c \"...\"` or edit in WSL) before `make bearhost-rsync` |

| RAM pressure | Lower caps in `profile-bearhost-prod.env`; `free -h` |

| OAuth on raw IP | Update Twitch redirect URIs or defer login test |

| Scraper scrape timeout | Check proxy env; increase `SCRAPER_TIMEOUT_MS` in smoke |

| Portal `/analytics` chart empty or hub 404 | Redeploy Pulse API analytics: `make bearhost-rsync` then `bash scripts/bearhost-pulse-redeploy-remote.sh`. Post-deploy: `PULSE_SMOKE_BASE_URL=https://api.streampulse.stream PULSE_EXPECT_HOSTED_MODE=true bash deploy/smoke/bearhost-pulse-api.sh` — gates `/v1/extension/health` and `GET /v1/public/hub?activityWindow=30m` (200, `activity.points.length >= 2`) |



Related: [ENVIRONMENT.md](ENVIRONMENT.md) · [scraping-archive/requirements.md](scraping-archive/requirements.md)
