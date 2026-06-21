#!/usr/bin/env bash
# Quarterly restore drill — rehydrate one archived stream without scrape.
# Cron: 0 4 1 1,4,7,10 * streamclone /opt/streamclone/app/scripts/bearhost-restore-drill-cron.sh
#
# Override schedule: BEARHOST_RESTORE_DRILL_FORCE=1 STREAM_ID=<id> bash scripts/bearhost-restore-drill-cron.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

# shellcheck source=scripts/lib/bearhost-compose.sh
source "${ROOT}/scripts/lib/bearhost-compose.sh"

LOG_DIR="${STREAMCLONE_CRON_LOG_DIR:-/opt/streamclone/backups/cron}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
LOG="${LOG_DIR}/restore-drill-${STAMP}.log"
mkdir -p "${LOG_DIR}"

exec >>"${LOG}" 2>&1

echo "==> restore drill cron $(date -u +%Y-%m-%dT%H:%M:%SZ)"

if [[ "${BEARHOST_RESTORE_DRILL_FORCE:-0}" != "1" ]]; then
  MONTH="$(date -u +%-m)"
  DAY="$(date -u +%-d)"
  case "${MONTH}" in
    1|4|7|10) ;;
    *)
      echo "bearhost-restore-drill-cron: skip (not a quarterly month)"
      exit 0
      ;;
  esac
  if [[ "${DAY}" != "1" ]]; then
    echo "bearhost-restore-drill-cron: skip (not day 1)"
    exit 0
  fi
fi

STREAM_ID="${STREAM_ID:-}"
if [[ -z "${STREAM_ID}" ]]; then
  STREAM_ID="$(
    bearhost_compose exec -T postgres psql -U app -d streamclone -Atqc \
      "SELECT stream_id FROM archive_job_items
       WHERE stream_id IS NOT NULL AND stream_id <> ''
       ORDER BY updated_at DESC LIMIT 1;" 2>/dev/null || true
  )"
fi

if [[ -z "${STREAM_ID}" ]]; then
  STREAM_ID="$(
    bearhost_compose exec -T postgres psql -U app -d streamclone -Atqc \
      "SELECT regexp_replace(natural_key, '^rollups:', '')
       FROM archive_exports
       WHERE artifact_type LIKE '%rollup%' AND natural_key LIKE 'rollups:%'
       ORDER BY updated_at DESC LIMIT 1;" 2>/dev/null || true
  )"
fi

if [[ -z "${STREAM_ID}" ]]; then
  echo "bearhost-restore-drill-cron: BLOCKED — no STREAM_ID and none in archive_job_items/archive_exports"
  exit 2
fi

echo "==> STREAM_ID=${STREAM_ID}"
BEARHOST_USE_DOCKER_GO="${BEARHOST_USE_DOCKER_GO:-1}" \
  STREAM_ID="${STREAM_ID}" \
  bash "${ROOT}/scripts/archive-restore-drill.sh"

echo "bearhost-restore-drill-cron: pass"
