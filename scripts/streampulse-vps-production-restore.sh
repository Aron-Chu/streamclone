#!/usr/bin/env bash
# Restore a BearHost pg_dump into streampulse-vps local Postgres (production SoT).
#
# Prerequisites:
#   - DUMP_PATH points at a custom-format pg_dump from backup-bearhost.sh
#   - streampulse-vps Postgres is up (empty or replaceable — operator confirms)
#   - STOP analytics-workers before restore if replacing live data
#
# Usage:
#   DUMP_PATH=runtime/backups/streamclone-bearhost-*.dump \
#     bash scripts/streampulse-vps-production-restore.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORKER="${WORKER:-23.173.152.156}"
WORKER_KEY="${WORKER_KEY:-${HOME}/.ssh/id_ed25519}"
WORKER_APP="${WORKER_APP:-/opt/streamclone/app}"
DUMP_PATH="${DUMP_PATH:-}"

if [[ -z "${DUMP_PATH}" || ! -f "${DUMP_PATH}" ]]; then
  echo "DUMP_PATH must point at an existing custom-format pg_dump" >&2
  exit 1
fi

ssh_worker() { ssh -i "${WORKER_KEY}" -o BatchMode=yes "root@${WORKER}" "$@"; }

REMOTE_DUMP="/tmp/$(basename "${DUMP_PATH}")"
echo "==> Upload dump to streampulse-vps:${REMOTE_DUMP}"
scp -i "${WORKER_KEY}" -o BatchMode=yes "${DUMP_PATH}" "root@${WORKER}:${REMOTE_DUMP}"

echo "==> Restore into local Postgres (pg_restore --clean --if-exists)"
ssh_worker bash -s <<REMOTE
set -euo pipefail
cd ${WORKER_APP}
docker compose -f deploy/docker-compose.yml up -d postgres
sleep 5
docker exec streamclone-postgres-1 pg_isready -U app -d streamclone
docker cp "${REMOTE_DUMP}" streamclone-postgres-1:/tmp/restore.dump
docker exec streamclone-postgres-1 pg_restore -U app -d streamclone --clean --if-exists --no-owner --no-acl /tmp/restore.dump
docker exec streamclone-postgres-1 rm -f /tmp/restore.dump
rm -f "${REMOTE_DUMP}"
echo "==> schema_migrations after restore:"
docker exec streamclone-postgres-1 psql -U app -d streamclone -P pager=off -c "SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 3;"
REMOTE

echo "restore done — run streampulse-vps-production-deploy.sh migrate+smoke before DNS cutover"
