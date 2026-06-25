#!/usr/bin/env bash
# Postgres table list + pulse/analytics table columns (stack must be up).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="${ROOT}/runtime/context"
mkdir -p "$OUT"
ENV_FILE="${ENV_FILE:-${ROOT}/.env}"
COMPOSE=(docker compose --env-file "$ENV_FILE" -f "${ROOT}/deploy/docker-compose.yml" -f "${ROOT}/deploy/docker-compose.local-tunnel.yml")

{
  echo "# DB schema snapshot — $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo
  if ! "${COMPOSE[@]}" ps postgres --status running -q 2>/dev/null | grep -q .; then
    echo "postgres: not running (make up)"
    exit 0
  fi
  echo "## Tables (public)"
  "${COMPOSE[@]}" exec -T postgres psql -U streamclone -d streamclone -c "\dt public.*" 2>/dev/null || echo "(psql failed)"
  echo
  echo "## Pulse / analytics highlights"
  for t in backfill_jobs pulse_bookmarks analytics_streams; do
    echo "### ${t}"
    "${COMPOSE[@]}" exec -T postgres psql -U streamclone -d streamclone -c "\d ${t}" 2>/dev/null || echo "(table ${t} missing or error)"
    echo
  done
} > "${OUT}/db_schema.txt"

echo "wrote ${OUT}/db_schema.txt"
