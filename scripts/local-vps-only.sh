#!/usr/bin/env bash
# Stop local scraper + disable Tier-0/Bronze/backfill — corpus runs on legacy-rollback-host only.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

# shellcheck source=scripts/lib/env.sh
source "${ROOT}/scripts/lib/env.sh"

overlay="${ROOT}/deploy/env/profile-local-vps-only.env"
local_env="${ROOT}/.env.local"
env_file="${ROOT}/.env"

if [[ ! -f "${env_file}" ]]; then
  cp .env.dev "${env_file}"
fi

if [[ -f "${overlay}" ]]; then
  if ! grep -q 'STREAMCLONE_DISABLE_LOCAL_SCRAPER' "${local_env}" 2>/dev/null; then
    {
      echo ''
      echo '# VPS-only scraping (see deploy/env/profile-local-vps-only.env)'
      cat "${overlay}"
    } >> "${local_env}"
  fi
  while IFS= read -r line || [[ -n "${line}" ]]; do
    case "${line}" in
      ''|'#'*) continue ;;
      *=*)
        key="${line%%=*}"
        value="${line#*=}"
        env_set_key "${env_file}" "${key}" "${value}"
        ;;
    esac
  done < "${overlay}"
fi

profiles="$(env_feature_compose_profiles "${env_file}")"
compose=(docker compose --env-file "${env_file}" -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml)
for p in ${profiles}; do
  compose+=(--profile "${p}")
done

echo "local-vps-only: stopping local scraper (if running)"
docker stop streamclone-scraper 2>/dev/null || true
docker rm -f streamclone-scraper 2>/dev/null || true

echo "local-vps-only: recreating analytics with workers disabled"
"${compose[@]}" up -d --force-recreate --no-deps analytics || {
  echo "local-vps-only: compose recreate failed — try from repo root:" >&2
  echo "  docker compose --env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml --profile pulse-wire up -d --no-deps analytics" >&2
  exit 1
}

echo "local-vps-only: done — scrape/bronze/tier-0 on VPS only"
echo "  check VPS: bash scripts/bearhost-bronze-status-remote.sh"
