#!/usr/bin/env bash
# Enable Grafana + Prometheus on BearHost VPS (from your PC via SSH).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"

bearhost_ssh_config
echo "==> rsync observability files to ${BEARHOST_HOST}"
bash "${ROOT}/scripts/bearhost-rsync-to-vps.sh"

echo "==> rebuild analytics-workers (archive metrics) + start observability"
bearhost_ssh "cd '${BEARHOST_REMOTE_APP}' && \
  BEARHOST_USE_DOCKER_GO=1 bash scripts/bearhost-observability.sh up && \
  docker compose --env-file .env --env-file deploy/env/profile-full.env --env-file deploy/env/profile-archive.env --env-file deploy/env/profile-bearhost-prod.env \
    -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml -f deploy/docker-compose.bearhost-prod.yml -f deploy/docker-compose.bearhost-build.yml \
    --profile scraper up -d --build analytics-workers"

echo ""
echo "Grafana ready on VPS — tunnel from your PC:"
echo "  make grafana-up"
echo "  open http://localhost:3001/d/streamclone-archive/streamclone-archive"
echo "  login: admin / streampulse (change on first visit)"
