#!/usr/bin/env bash
# Nightly Postgres backup for BearHost production.
# Cron: 0 3 * * * streamclone /opt/streamclone/app/scripts/bearhost-pg-backup.sh
#
# Writes gzip dumps to /opt/streamclone/backups/ with 14-day retention.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# shellcheck source=scripts/lib/bearhost-compose.sh
source "${ROOT}/scripts/lib/bearhost-compose.sh"

BACKUP_DIR="${STREAMCLONE_BACKUPS_DIR:-/opt/streamclone/backups}"
RETENTION_DAYS="${BEARHOST_BACKUP_RETENTION_DAYS:-14}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT="${BACKUP_DIR}/streamclone-${STAMP}.sql.gz"

mkdir -p "${BACKUP_DIR}"

echo "bearhost-pg-backup: dumping to ${OUT}"
bearhost_compose exec -T postgres pg_dump -U app -d streamclone --no-owner --no-acl | gzip > "${OUT}"

if [[ ! -s "${OUT}" ]]; then
  echo "bearhost-pg-backup: dump file is empty" >&2
  exit 1
fi

echo "bearhost-pg-backup: pruning backups older than ${RETENTION_DAYS} days"
find "${BACKUP_DIR}" -name 'streamclone-*.sql.gz' -type f -mtime +"${RETENTION_DAYS}" -delete

echo "bearhost-pg-backup: done ($(du -h "${OUT}" | awk '{print $1}'))"

if [[ "${BEARHOST_PG_BACKUP_SKIP_AZURE_UPLOAD:-0}" != "1" ]]; then
  # shellcheck source=scripts/lib/bearhost-azure-pg-backup-upload.sh
  source "${ROOT}/scripts/lib/bearhost-azure-pg-backup-upload.sh"
  if ! bearhost_upload_pg_backup_to_azure "${OUT}"; then
    echo "bearhost-pg-backup: offsite upload failed (local dump retained at ${OUT})" >&2
    exit 1
  fi
else
  echo "bearhost-pg-backup: offsite upload skipped (BEARHOST_PG_BACKUP_SKIP_AZURE_UPLOAD=1)"
fi
