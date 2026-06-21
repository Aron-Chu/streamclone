#!/usr/bin/env bash
# Install recommended BearHost production cron entries for the streamclone user.
# Usage: bash scripts/bearhost-cron-install.sh [--dry-run]
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP="${STREAMCLONE_APP_DIR:-/opt/streamclone/app}"
DRY_RUN=0

if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=1
fi

MARKER="# streamclone-bearhost-ops"
CRON_BLOCK="${MARKER}
0 3 * * * ${APP}/scripts/bearhost-pg-backup.sh
30 5 * * * ${APP}/scripts/bearhost-archive-coverage-cron.sh
0 4 1 1,4,7,10 * ${APP}/scripts/bearhost-restore-drill-cron.sh"

existing="$(crontab -l 2>/dev/null || true)"
if printf '%s\n' "${existing}" | grep -qF "${MARKER}"; then
  echo "bearhost-cron-install: ops block already present (marker ${MARKER})"
  exit 0
fi

if [[ "${DRY_RUN}" == "1" ]]; then
  echo "bearhost-cron-install: dry-run — would append:"
  printf '%s\n' "${CRON_BLOCK}"
  exit 0
fi

{
  if [[ -n "${existing}" ]]; then
    printf '%s\n' "${existing}"
  fi
  printf '%s\n' "${CRON_BLOCK}"
} | crontab -

echo "bearhost-cron-install: installed nightly backup, daily coverage, quarterly restore drill"
echo "  verify: crontab -l | grep streamclone-bearhost-ops"
