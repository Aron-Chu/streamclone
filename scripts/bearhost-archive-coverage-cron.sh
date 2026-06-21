#!/usr/bin/env bash
# Daily archive coverage snapshot for BearHost cron.
# Cron: 30 5 * * * streamclone /opt/streamclone/app/scripts/bearhost-archive-coverage-cron.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

REPORT_DIR="${STREAMCLONE_COVERAGE_DIR:-/opt/streamclone/backups/coverage}"
STAMP="$(date -u +%Y-%m-%d)"
OUT="${REPORT_DIR}/coverage-${STAMP}.json"
LOG_DIR="${STREAMCLONE_CRON_LOG_DIR:-/opt/streamclone/backups/cron}"
LOG="${LOG_DIR}/coverage-${STAMP}.log"

mkdir -p "${REPORT_DIR}" "${LOG_DIR}"

run_go() {
  if [[ "${BEARHOST_USE_DOCKER_GO:-0}" == "1" ]] || ! command -v go >/dev/null 2>&1; then
    BEARHOST_USE_DOCKER_GO=1 bash "${ROOT}/scripts/bearhost-go-run.sh" "$@"
  else
    go run "$@"
  fi
}

{
  echo "==> coverage report $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  run_go ./cmd/backfill coverage report --since=7d --out="${OUT}"
  echo "==> stale jobs snapshot"
  run_go ./cmd/backfill jobs list --status=stale --limit=20
  echo "bearhost-archive-coverage-cron: pass"
} >>"${LOG}" 2>&1
