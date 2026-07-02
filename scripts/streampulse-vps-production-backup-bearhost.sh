#!/usr/bin/env bash
# Dump BearHost production Postgres (custom format) for streampulse-vps migration.
# Read-only on BearHost — does not stop services or mutate application data.
#
# Usage:
#   bash scripts/streampulse-vps-production-backup-bearhost.sh
#   BACKUP_OUT=/tmp/streamclone-bearhost.dump bash scripts/streampulse-vps-production-backup-bearhost.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
REMOTE_DUMP="/tmp/streamclone-bearhost-${STAMP}.dump"
BACKUP_OUT="${BACKUP_OUT:-${ROOT}/runtime/backups/streamclone-bearhost-${STAMP}.dump}"

mkdir -p "$(dirname "${BACKUP_OUT}")"

echo "==> BearHost: pg_dump custom format -> ${REMOTE_DUMP}"
bearhost_ssh_config
bearhost_ssh bash -s <<REMOTE
set -euo pipefail
docker exec streamclone-postgres-1 pg_dump -U app -d streamclone -Fc --no-owner --no-acl -f /tmp/bearhost-migrate.dump
docker cp streamclone-postgres-1:/tmp/bearhost-migrate.dump "${REMOTE_DUMP}"
ls -lh "${REMOTE_DUMP}"
REMOTE

echo "==> Download dump to ${BACKUP_OUT}"
scp -i "${BEARHOST_SSH_KEY}" -o BatchMode=yes \
  "${BEARHOST_USER}@${BEARHOST_HOST}:${REMOTE_DUMP}" "${BACKUP_OUT}"

echo "==> BearHost: remove remote temp dump"
bearhost_ssh "rm -f ${REMOTE_DUMP}" || true

echo "backup done: ${BACKUP_OUT} ($(du -h "${BACKUP_OUT}" | awk '{print $1}'))"
echo "Next: scp dump to streampulse-vps and run streampulse-vps-production-restore.sh"
