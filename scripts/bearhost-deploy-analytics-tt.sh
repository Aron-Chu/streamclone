#!/usr/bin/env bash
# Rebuild analytics + workers after TT scrape/backoff changes; reload observability configs.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"

bearhost_ssh_config
REMOTE='cd /opt/streamclone/app && source scripts/lib/bearhost-compose.sh
echo "==> build analytics + analytics-workers"
bearhost_compose build analytics analytics-workers
echo "==> restart analytics plane"
bearhost_compose up -d analytics analytics-workers
bearhost_compose up -d --force-recreate --no-deps analytics-workers
echo "==> reload observability (Grafana/Prometheus configs)"
bash scripts/bearhost-observability.sh up || true
echo "==> service status"
bearhost_compose ps analytics analytics-workers scraper postgres redis
echo "==> workers log tail"
docker logs --tail=25 streamclone-analytics-workers 2>&1'

echo "==> bearhost-deploy-analytics-tt on ${BEARHOST_USER}@${BEARHOST_HOST}"
bearhost_ssh "${REMOTE}"

echo "bearhost-deploy-analytics-tt: done"
