#!/usr/bin/env bash
# Read-only Step 0 baseline for ingest-core Phase C shadow validation.
# Run on the production VPS via SSH (streampulse-ops). No deploy/restart/env edits.
set -euo pipefail

OUT_DIR="${1:-runtime/evidence/ingest-core-phase-c-step0-$(date -u +%Y%m%dT%H%M%SZ)}"
mkdir -p "${OUT_DIR}"

log() { echo "$*" | tee -a "${OUT_DIR}/commands.log"; }

docker_env() {
  local container="$1"
  local var="$2"
  docker inspect "${container}" --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null \
    | grep -E "^${var}=" | head -1 | cut -d= -f2- || echo unset
}

REDIS="$(docker ps --format '{{.Names}}' | grep -E 'redis' | head -1 || true)"
redis_cli() {
  if [[ -n "${REDIS}" ]]; then
    docker exec "${REDIS}" redis-cli "$@"
  else
    redis-cli "$@"
  fi
}

log "==> Step 0 baseline $(date -Is)"
log "output_dir=${OUT_DIR}"

{
  date -Is
  docker ps
  docker stats --no-stream
  redis_cli INFO stats
  redis_cli INFO memory
  redis_cli INFO clients
  df -h
  free -h
} > "${OUT_DIR}/vps-host.txt" 2>&1 || true

ANALYTICS="$(docker ps --format '{{.Names}}' | grep -E 'analytics' | grep -v workers | head -1 || true)"
if [[ -n "${ANALYTICS}" ]]; then
  {
    echo "container=${ANALYTICS}"
    echo "image=$(docker inspect "${ANALYTICS}" --format '{{.Config.Image}}')"
    for var in STREAMCLONE_VERSION IMAGE_TAG INGEST_CORE_ENABLED INGEST_CORE_DUAL_READ_MODE \
      INGEST_CORE_SHADOW_MODE CORPUS_WORKERS_ENABLED SCRAPER_ENABLED_ON_API_NODE; do
      echo "${var}=$(docker_env "${ANALYTICS}" "${var}")"
    done
  } > "${OUT_DIR}/rollback-anchors-redacted.txt" 2>&1
fi

PG="$(docker ps --format '{{.Names}}' | grep -E 'postgres' | head -1 || true)"
if [[ -n "${PG}" ]]; then
  PG_USER="$(docker_env "${PG}" POSTGRES_USER)"
  PG_DB="$(docker_env "${PG}" POSTGRES_DB)"
  [[ "${PG_USER}" == unset ]] && PG_USER="app"
  [[ "${PG_DB}" == unset ]] && PG_DB="streamclone"
  docker exec "${PG}" psql -U "${PG_USER}" -d "${PG_DB}" -c \
    "SELECT pg_size_pretty(pg_database_size(current_database())) AS db_size;" \
    > "${OUT_DIR}/postgres-db-size.txt" 2>&1 || true
  docker exec "${PG}" psql -U "${PG_USER}" -d "${PG_DB}" -c \
    "SELECT schemaname, relname, pg_size_pretty(pg_total_relation_size(relid)) AS total_size, n_live_tup, n_dead_tup FROM pg_stat_user_tables ORDER BY pg_total_relation_size(relid) DESC LIMIT 20;" \
    > "${OUT_DIR}/postgres-top-tables.txt" 2>&1 || true
fi

PROM="${PROMETHEUS_URL:-http://127.0.0.1:9090}"
if curl -fsS "${PROM}/api/v1/status/config" >/dev/null 2>&1; then
  for q in \
    'ingest_active_collectors' \
    'ingest_desired_collectors' \
    'rate(ingest_messages_dropped_total[5m])' \
    'ingest_flush_queue_depth' \
    'histogram_quantile(0.95,rate(analytics_rollup_write_duration_seconds_bucket[5m]))'; do
    curl -sG "${PROM}/api/v1/query" --data-urlencode "query=${q}" > "${OUT_DIR}/prom-$(echo "${q}" | tr '/()[]:' '_____' | cut -c1-40).json" 2>/dev/null || true
  done
  echo "prometheus=reachable" >> "${OUT_DIR}/prometheus-status.txt"
else
  echo "prometheus=unreachable (metric absent pre-release for ingest_* is OK before shadow image)" > "${OUT_DIR}/prometheus-status.txt"
fi

BASE="${PUBLIC_API_BASE:-https://api.streampulse.stream}"
{
  curl -sI "${BASE}/v1/public/hub?activityWindow=24h"
  echo "---"
  bucketT=$(( ($(date +%s)/300*300 - 600) * 1000 ))
  curl -sI "${BASE}/v1/public/hub/moments?bucketT=${bucketT}&activityWindow=1440"
  echo "---"
  curl -s "${BASE}/v1/extension/health"
  echo "---"
  curl -s "${BASE}/v1/public/hub?activityWindow=24h" | jq '{ingest,coverage,corpusPipeline:{state:.corpusPipeline.state,collectorActive:.corpusPipeline.collectorActive,collectorMax:.corpusPipeline.collectorMax}}'
} > "${OUT_DIR}/public-api.txt" 2>&1 || true

log "DONE: ${OUT_DIR}"
log "Next: review summary checklist in docs/pulse-ingest-v2/ingest-core-runbook.md"
