#!/usr/bin/env bash
# Disposable Postgres restore smoke — local/staging only. Never targets production DB name.
# Usage: DUMP=/path/to/streamclone-*.sql.gz bash scripts/pg-restore-smoke-disposable.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# shellcheck source=scripts/lib/env.sh
source "${ROOT}/scripts/lib/env.sh"

ENV_FILE="${ENV_FILE:-.env}"
COMPOSE=(docker compose --env-file "${ENV_FILE}")
if [[ -f .env.local ]]; then
  COMPOSE+=(--env-file .env.local)
fi
COMPOSE+=(-f deploy/docker-compose.yml)
if [[ "${PG_RESTORE_SMOKE_USE_LOCAL_TUNNEL:-0}" == "1" ]]; then
  COMPOSE+=(-f deploy/docker-compose.local-tunnel.yml)
fi

DB_SMOKE="${PG_RESTORE_SMOKE_DB:-streamclone_restore_smoke}"
DUMP="${DUMP:-}"

if [[ -z "${DUMP}" || ! -s "${DUMP}" ]]; then
  echo "pg-restore-smoke: set DUMP to a non-empty .sql.gz path" >&2
  exit 2
fi

if [[ "${DB_SMOKE}" == "streamclone" ]]; then
  echo "pg-restore-smoke: refusing to restore into production database name" >&2
  exit 2
fi

log() { printf 'pg-restore-smoke: %s\n' "$*"; }

"${COMPOSE[@]}" up -d postgres >/dev/null
"${COMPOSE[@]}" exec -T postgres pg_isready -U app -d postgres >/dev/null

log "terminating connections to ${DB_SMOKE} if present"
"${COMPOSE[@]}" exec -T postgres psql -U app -d postgres -v ON_ERROR_STOP=1 -c \
  "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '${DB_SMOKE}';" >/dev/null 2>&1 || true
"${COMPOSE[@]}" exec -T postgres psql -U app -d postgres -v ON_ERROR_STOP=1 -c \
  "DROP DATABASE IF EXISTS ${DB_SMOKE};"
"${COMPOSE[@]}" exec -T postgres psql -U app -d postgres -v ON_ERROR_STOP=1 -c \
  "CREATE DATABASE ${DB_SMOKE};"

start_ms="$(date +%s)"
log "restoring ${DUMP} -> ${DB_SMOKE}"
gunzip -c "${DUMP}" | "${COMPOSE[@]}" exec -T postgres psql -U app -d "${DB_SMOKE}" -v ON_ERROR_STOP=1 -q
end_ms="$(date +%s)"
duration="$((end_ms - start_ms))"
log "restore duration: ${duration}s"

log "sanity checks"
"${COMPOSE[@]}" exec -T postgres psql -U app -d "${DB_SMOKE}" -v ON_ERROR_STOP=1 -c \
  "SELECT 'analytics_streams' AS tbl, count(*)::bigint AS rows FROM analytics_streams
   UNION ALL SELECT 'analytics_minute_rollups', count(*)::bigint FROM analytics_minute_rollups
   UNION ALL SELECT 'archive_exports', count(*)::bigint FROM archive_exports;"
"${COMPOSE[@]}" exec -T postgres psql -U app -d "${DB_SMOKE}" -Atqc \
  "select pg_size_pretty(pg_database_size(current_database()));"

log "dropping disposable database ${DB_SMOKE}"
"${COMPOSE[@]}" exec -T postgres psql -U app -d postgres -v ON_ERROR_STOP=1 -c \
  "DROP DATABASE ${DB_SMOKE};"

log "PASS disposable restore smoke (duration=${duration}s)"
