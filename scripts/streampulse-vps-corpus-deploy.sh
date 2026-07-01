#!/usr/bin/env bash
# Deploy Silver/Gold corpus workers (scraper + analytics-workers) on streampulse-vps.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BEARHOST="${BEARHOST:-141.11.243.103}"
WORKER="${WORKER:-23.173.152.156}"
BEARHOST_KEY="${BEARHOST_KEY:-${HOME}/.ssh/id_ed25519_bearhost_streamclone}"
WORKER_KEY="${WORKER_KEY:-${HOME}/.ssh/id_ed25519}"
# WSL: when invoked as root, prefer the interactive user's keys.
if [[ ! -f "${BEARHOST_KEY}" && -f "/home/aron/.ssh/id_ed25519_bearhost_streamclone" ]]; then
  BEARHOST_KEY="/home/aron/.ssh/id_ed25519_bearhost_streamclone"
  WORKER_KEY="/home/aron/.ssh/id_ed25519"
fi
BEARHOST_APP="${BEARHOST_APP:-/opt/streamclone/app}"
WORKER_APP="${WORKER_APP:-/opt/streamclone/app}"
SKIP_QUEUE_SNAPSHOT="${SKIP_QUEUE_SNAPSHOT:-0}"
CANARY_GOLD_VOD_SEGMENTS="${CANARY_GOLD_VOD_SEGMENTS:-0}"
GOLD_VOD_SEGMENTS_FLAG=false
GQL_CONCURRENCY=1
if [[ "${CANARY_GOLD_VOD_SEGMENTS}" == "1" || "${CANARY_GOLD_VOD_SEGMENTS}" == "true" ]]; then
  GOLD_VOD_SEGMENTS_FLAG=true
  # Durable segment ledger requires parallel GQL (see sync_gql_parallel.go).
  GQL_CONCURRENCY=2
fi

ssh_bearhost() { ssh -i "${BEARHOST_KEY}" -o BatchMode=yes "root@${BEARHOST}" "$@"; }
ssh_worker() { ssh -i "${WORKER_KEY}" -o BatchMode=yes "root@${WORKER}" "$@"; }

if [[ "${SKIP_QUEUE_SNAPSHOT}" != "1" ]]; then
  echo "==> Snapshot backfill_jobs (pre-deploy)"
  ssh_bearhost "cd ${BEARHOST_APP} && bash scripts/bearhost-silver-gold-pg.sh snapshot" || true
fi

echo "==> streampulse-vps: rsync local repo"
ssh_worker "apt-get update -qq; DEBIAN_FRONTEND=noninteractive apt-get install -y -qq docker.io rsync; mkdir -p ${WORKER_APP}"
rsync -avz \
  --exclude .git \
  --exclude node_modules \
  --exclude frontend/node_modules \
  --exclude .env \
  --exclude .env.local \
  --exclude runtime \
  --exclude pg-data \
  --exclude .codegraph \
  --exclude .uv-python \
  --exclude dist \
  -e "ssh -i ${WORKER_KEY} -o BatchMode=yes" \
  "${ROOT}/" "root@${WORKER}:${WORKER_APP}/"

SCRAPER_ROOT="${SCRAPER_ROOT:-$(cd "${ROOT}/../streamclone-scraper" 2>/dev/null && pwd || true)}"
if [[ -z "${SCRAPER_ROOT}" || ! -d "${SCRAPER_ROOT}" ]]; then
  SCRAPER_ROOT="${SCRAPER_ROOT:-/mnt/c/Users/Aron/streamclone-scraper}"
fi
if [[ -d "${SCRAPER_ROOT}" ]]; then
  echo "==> streampulse-vps: rsync streamclone-scraper (${SCRAPER_ROOT})"
  ssh_worker "mkdir -p /opt/streamclone/streamclone-scraper"
  rsync -avz \
    --exclude .git \
    --exclude node_modules \
    --exclude __pycache__ \
    -e "ssh -i ${WORKER_KEY} -o BatchMode=yes" \
    "${SCRAPER_ROOT}/" "root@${WORKER}:/opt/streamclone/streamclone-scraper/"
else
  echo "WARN: streamclone-scraper not found at ${SCRAPER_ROOT}; scraper build may fail" >&2
fi

echo "==> Build corpus local.env on BearHost (Tailscale URLs; GOLD_VOD_SEGMENTS_ENABLED=${GOLD_VOD_SEGMENTS_FLAG})"
ssh_bearhost "bash -s" <<BEARHOST_ENV
set -euo pipefail
ENV_FILE=${BEARHOST_APP}/.env
read_env() {
  local key="\$1"
  grep "^\${key}=" "\${ENV_FILE}" 2>/dev/null | head -1 | cut -d= -f2- || true
}
DATABASE_URL="\$(read_env DATABASE_URL)"
REDIS_URL="\$(read_env REDIS_URL)"
SCRAPER_API_KEY="\$(read_env SCRAPER_API_KEY)"
TWITCH_OAUTH_CLIENT_ID="\$(read_env TWITCH_OAUTH_CLIENT_ID)"
TWITCH_OAUTH_CLIENT_SECRET="\$(read_env TWITCH_OAUTH_CLIENT_SECRET)"
DB_URL="\${DATABASE_URL/@postgres:/@bearhost:}"
DB_URL="\${DB_URL/postgres:\/\/postgres/postgres:\/\/bearhost}"
REDIS_OUT="\${REDIS_URL/redis:\/\/redis/redis:\/\/bearhost}"
REDIS_OUT="\${REDIS_OUT/@redis:/@bearhost:}"
cat > /tmp/corpus.local.env <<EOF
LOG_LEVEL=info
DATABASE_URL=\${DB_URL}
REDIS_URL=\${REDIS_OUT}
TWITCH_OAUTH_CLIENT_ID=\${TWITCH_OAUTH_CLIENT_ID}
TWITCH_OAUTH_CLIENT_SECRET=\${TWITCH_OAUTH_CLIENT_SECRET}
EMOTE_SERVICE_URL=http://bearhost:8084
SCRAPER_API_KEY=\${SCRAPER_API_KEY}
SCRAPER_MAX_CONCURRENT=1
CORPUS_WORKERS_ENABLED=1
BACKFILL_ENABLED=true
GOLD_BACKFILL_ENABLED=true
BACKFILL_GOLD_WORKER_COUNT=1
BACKFILL_SILVER_WORKER_COUNT=1
BACKFILL_REQUEUE_FAILED_MAX_PER_RUN=25
SILVER_AUTO_ENQUEUE_ENABLED=true
ANALYTICS_VOD_GQL_CONCURRENCY=${GQL_CONCURRENCY}
ANALYTICS_VOD_GQL_CONCURRENCY_MAX=4
ARCHIVE_ENABLED=false
BRONZE_ENABLED=false
PULSE_COLLECTOR_ENABLED=false
PULSE_AUTO_BACKFILL_ENABLED=false
GOLD_VOD_SEGMENTS_ENABLED=${GOLD_VOD_SEGMENTS_FLAG}
EOF
chmod 600 /tmp/corpus.local.env
BEARHOST_ENV

scp -i "${BEARHOST_KEY}" -o BatchMode=yes "root@${BEARHOST}:/tmp/corpus.local.env" /tmp/corpus.local.env.streampulse
scp -i "${WORKER_KEY}" -o BatchMode=yes /tmp/corpus.local.env.streampulse "root@${WORKER}:${WORKER_APP}/deploy/env/profile-streampulse-vps-corpus.local.env"
rm -f /tmp/corpus.local.env.streampulse

echo "==> streampulse-vps: build + start corpus workers (GQL concurrency=${GQL_CONCURRENCY}; GOLD_VOD_SEGMENTS_ENABLED=${GOLD_VOD_SEGMENTS_FLAG})"
ssh_worker "cd ${WORKER_APP} && docker compose -f deploy/docker-compose.streampulse-vps-corpus.yml up -d --build"

echo "==> Waiting for health..."
sleep 25
ssh_worker "docker ps --filter name=streampulse- --format '{{.Names}} {{.Status}}'; docker logs streampulse-analytics-workers --tail 30"

echo "==> Hub corpus pipeline (Silver/Gold tier counts)"
curl -sf "https://api.streampulse.stream/v1/public/hub" | head -c 2000 || true
echo

echo "==> Done. Rollback: ssh worker && docker compose -f deploy/docker-compose.streampulse-vps-corpus.yml down"
