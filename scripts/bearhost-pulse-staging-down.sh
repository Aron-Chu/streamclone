#!/usr/bin/env bash
# Stop isolated Pulse staging stack (127.0.0.1:8091).
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

echo "==> bearhost-pulse-staging-down"
bearhost_compose_staging down
