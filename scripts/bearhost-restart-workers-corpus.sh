#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"
bearhost_ssh_config
scp -i "${BEARHOST_SSH_KEY}" -o StrictHostKeyChecking=accept-new \
  "${ROOT}/deploy/docker-compose.bearhost-prod.yml" \
  "${ROOT}/deploy/env/profile-bearhost-corpus.env" \
  "${BEARHOST_USER}@${BEARHOST_HOST}:/opt/streamclone/app/deploy/"
scp -i "${BEARHOST_SSH_KEY}" -o StrictHostKeyChecking=accept-new \
  "${ROOT}/deploy/env/profile-bearhost-corpus.env" \
  "${BEARHOST_USER}@${BEARHOST_HOST}:/opt/streamclone/app/deploy/env/"
bearhost_ssh "cd /opt/streamclone/app && docker compose \
  --env-file .env \
  --env-file deploy/env/profile-full.env \
  --env-file deploy/env/profile-archive.env \
  --env-file deploy/env/profile-bearhost-prod.env \
  --env-file deploy/env/profile-bearhost-corpus.env \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.prod.yml \
  -f deploy/docker-compose.bearhost-prod.yml \
  -f deploy/docker-compose.bearhost-build.yml \
  --profile scraper up -d --build analytics-workers"
bearhost_ssh "docker logs --tail=15 streamclone-analytics-workers 2>&1 | grep -E 'silver|backfill|bronze' || docker logs --tail=8 streamclone-analytics-workers 2>&1"
