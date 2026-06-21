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



## Nightly backup



```bash

# crontab -u streamclone -e

0 3 * * * /opt/streamclone/app/scripts/bearhost-pg-backup.sh

```



Dumps: `/opt/streamclone/backups/streamclone-*.sql.gz` (14-day retention).



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



Related: [ENVIRONMENT.md](ENVIRONMENT.md) · [scraping-archive/requirements.md](scraping-archive/requirements.md)
