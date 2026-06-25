#!/usr/bin/env bash
# Start isolated Pulse staging stack on 127.0.0.1:8091 (LOAD-001b).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

# shellcheck source=scripts/lib/bearhost-compose.sh
source "${ROOT}/scripts/lib/bearhost-compose.sh"

bearhost_compose_staging() {
  local root
  root="$(bearhost_root_dir)"
  local args=(
    docker compose
    --env-file "${root}/.env"
    --env-file "${root}/deploy/env/profile-full.env"
    --env-file "${root}/deploy/env/profile-bearhost-prod.env"
    --env-file "${root}/deploy/env/profile-bearhost-pulse-staging.env"
  )
  local f
  while IFS= read -r f; do
    args+=("${f}")
  done < <(bearhost_compose_files)
  args+=(-f "${root}/deploy/docker-compose.bearhost-pulse-staging.yml")
  args+=("$@")
  "${args[@]}"
}

if [[ -f /etc/streamclone/secrets/pulse-beta.env ]]; then
  set -a
  # shellcheck disable=SC1091
  source /etc/streamclone/secrets/pulse-beta.env
  set +a
  echo "==> loaded /etc/streamclone/secrets/pulse-beta.env"
fi

KEEP=(
  postgres redis migrate metadata analytics emote minio pulse-caddy
)

echo "==> bearhost-pulse-staging-up: isolated stack on 127.0.0.1:8091 (cap 25)"
bearhost_compose_staging up -d "${KEEP[@]}"
bearhost_compose_staging up -d --force-recreate --no-deps analytics pulse-caddy

echo ""
echo "==> staging health"
curl -sf "http://127.0.0.1:8091/v1/extension/health" | head -c 400 || true
echo ""
echo "==> done — target PULSE_LOAD_TARGET=http://127.0.0.1:8091"
