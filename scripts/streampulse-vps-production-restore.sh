#!/usr/bin/env bash
# Restore a BearHost pg_dump into streampulse-vps production Postgres (streamclone-production project).
#
# Prerequisites:
#   - DUMP_PATH points at a custom-format pg_dump from backup-bearhost.sh
#   - STREAMPULSE_PG_RESTORE_CONFIRM=YES_I_HAVE_PROD_BACKUP
#   - analytics and analytics-workers must be stopped on the target VPS
#
# Usage:
#   STREAMPULSE_PG_RESTORE_CONFIRM=YES_I_HAVE_PROD_BACKUP \
#     DUMP_PATH=runtime/backups/streamclone-bearhost-*.dump \
#     bash scripts/streampulse-vps-production-restore.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/streampulse-vps-production-compose.sh
source "${ROOT}/scripts/lib/streampulse-vps-production-compose.sh"

WORKER="${WORKER:-23.173.152.156}"
WORKER_APP="${WORKER_APP:-/opt/streamclone/app}"
DUMP_PATH="${DUMP_PATH:-}"

streampulse_vps_resolve_worker_key
ssh_worker() { ssh -i "${WORKER_KEY}" -o BatchMode=yes "root@${WORKER}" "$@"; }

if [[ "${STREAMPULSE_PG_RESTORE_CONFIRM:-}" != "YES_I_HAVE_PROD_BACKUP" ]]; then
  echo "Refusing restore: set STREAMPULSE_PG_RESTORE_CONFIRM=YES_I_HAVE_PROD_BACKUP" >&2
  exit 1
fi

if [[ -z "${DUMP_PATH}" || ! -f "${DUMP_PATH}" ]]; then
  echo "DUMP_PATH must point at an existing custom-format pg_dump" >&2
  exit 1
fi

REMOTE_DUMP="/tmp/$(basename "${DUMP_PATH}")"
echo "==> Upload dump to streampulse-vps:${REMOTE_DUMP}"
scp -i "${WORKER_KEY}" -o BatchMode=yes "${DUMP_PATH}" "root@${WORKER}:${REMOTE_DUMP}"

echo "==> Restore into streamclone-production Postgres volume"
ssh_worker bash -s <<REMOTE
set -euo pipefail
cd ${WORKER_APP}
# shellcheck source=scripts/lib/streampulse-vps-production-compose.sh
source scripts/lib/streampulse-vps-production-compose.sh
ENV_LOCAL="\$(streampulse_vps_production_env_local)"
if [[ ! -f "\${ENV_LOCAL}" ]]; then
  echo "missing \${ENV_LOCAL} — copy from deploy/env/profile-streampulse-vps-production.env.example" >&2
  exit 1
fi

for svc in analytics analytics-workers; do
  if cid="\$(streampulse_vps_production_compose "${WORKER_APP}" ps -q "\${svc}" 2>/dev/null)" && [[ -n "\${cid}" ]]; then
    echo "Refusing restore: production \${svc} is running (cid=\${cid}). Stop it first." >&2
    exit 1
  fi
done

echo "==> target project: streamclone-production"
streampulse_vps_production_compose "${WORKER_APP}" config --services | head -5
streampulse_vps_production_compose "${WORKER_APP}" up -d postgres
sleep 5
pg_cid="\$(streampulse_vps_production_compose "${WORKER_APP}" ps -q postgres)"
if [[ -z "\${pg_cid}" ]]; then
  echo "postgres container not found in streamclone-production project" >&2
  exit 1
fi
docker exec "\${pg_cid}" pg_isready -U app -d streamclone
docker cp "${REMOTE_DUMP}" "\${pg_cid}:/tmp/restore.dump"
docker exec "\${pg_cid}" pg_restore -U app -d streamclone --clean --if-exists --no-owner --no-acl /tmp/restore.dump
docker exec "\${pg_cid}" rm -f /tmp/restore.dump
rm -f "${REMOTE_DUMP}"
echo "==> schema_migrations after restore:"
docker exec "\${pg_cid}" psql -U app -d streamclone -P pager=off -c "SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 3;"
REMOTE

echo "restore done — run streampulse-vps-production-deploy.sh migrate+smoke before DNS cutover"
