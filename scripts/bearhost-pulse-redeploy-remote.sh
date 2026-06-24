#!/usr/bin/env bash
# Sync is separate (bearhost-rsync-to-vps.sh). This rebuilds analytics + recreates Pulse API.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"
bearhost_ssh_config
echo "==> bearhost-pulse-redeploy on ${BEARHOST_USER}@${BEARHOST_HOST}"
bearhost_ssh bash -s <<'REMOTE'
set -euo pipefail
cd /opt/streamclone/app
ROOT=/opt/streamclone/app
# shellcheck source=scripts/lib/bearhost-compose.sh
source "${ROOT}/scripts/lib/bearhost-compose.sh"

pulse_compose() {
  local args=(
    docker compose
    --env-file "${ROOT}/.env"
    --env-file "${ROOT}/deploy/env/profile-full.env"
    --env-file "${ROOT}/deploy/env/profile-bearhost-prod.env"
    --env-file "${ROOT}/deploy/env/profile-bearhost-pulse.env"
  )
  local f
  while IFS= read -r f; do args+=("${f}"); done < <(BEARHOST_ROOT="${ROOT}" bearhost_compose_files)
  args+=(-f "${ROOT}/deploy/docker-compose.bearhost-pulse.yml")
  args+=("$@")
  "${args[@]}"
}

echo "==> migrate (forward-only)"
pulse_compose up -d migrate
pulse_compose wait migrate 2>/dev/null || sleep 8

echo "==> build analytics"
pulse_compose build analytics

echo "==> recreate pulse API stack"
bash scripts/bearhost-pulse-api.sh

echo "==> smoke localhost"
PULSE_SMOKE_BASE_URL=http://127.0.0.1:8090 PULSE_EXPECT_HOSTED_MODE=true \
  bash deploy/smoke/bearhost-pulse-api.sh
REMOTE

echo ""
echo "==> public health check"
curl -sf "https://api.streampulse.stream/v1/extension/health" | python3 -m json.tool 2>/dev/null || \
  curl -sf "https://api.streampulse.stream/v1/extension/health"
echo ""
