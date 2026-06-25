#!/usr/bin/env bash
# Silver/gold backfill_jobs + extension health (compact).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="${ROOT}/runtime/context"
BASE="${STREAMCLONE_BASE_URL:-http://localhost:8090}"
mkdir -p "$OUT"
ENV_FILE="${ENV_FILE:-${ROOT}/.env}"
COMPOSE=(docker compose --env-file "$ENV_FILE" -f "${ROOT}/deploy/docker-compose.yml" -f "${ROOT}/deploy/docker-compose.local-tunnel.yml")

{
  echo "# Backfill / pulse status — $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo
  echo "## Extension health"
  curl -sf --max-time 5 "${BASE}/v1/extension/health" 2>/dev/null | head -c 400 || echo "(health unreachable at ${BASE})"
  echo
  echo
  if "${COMPOSE[@]}" ps postgres --status running -q 2>/dev/null | grep -q .; then
    echo "## backfill_jobs (active + recent)"
    "${COMPOSE[@]}" exec -T postgres psql -U streamclone -d streamclone -c \
      "SELECT id, tier, login, status, export_status, updated_at FROM backfill_jobs ORDER BY updated_at DESC LIMIT 15;" 2>/dev/null \
      || echo "(query failed)"
  else
    echo "postgres: not running"
  fi
  echo
  echo "## Note"
  echo "Pulse VOD backfill jobs (extension) live in Redis/memory — use GET /v1/extension/pulse/backfill/{jobId} per job."
} > "${OUT}/backfill_status.txt"

echo "wrote ${OUT}/backfill_status.txt"
