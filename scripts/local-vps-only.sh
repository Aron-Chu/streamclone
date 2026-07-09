#!/usr/bin/env bash
# Stop local scraper when corpus runs on a remote worker only.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

# shellcheck source=scripts/lib/env.sh
source "${ROOT}/scripts/lib/env.sh"

overlay="${ROOT}/deploy/env/profile-local-vps-only.env"
local_env="${ROOT}/.env.local"
env_file="${ROOT}/.env"

if [[ ! -f "${env_file}" ]]; then
  bash scripts/env-synthesize.sh core "${env_file}"
fi

if [[ -f "${overlay}" ]]; then
  if ! grep -q 'STREAMCLONE_DISABLE_LOCAL_SCRAPER' "${local_env}" 2>/dev/null; then
    {
      echo ''
      echo '# Remote-worker scraping (see deploy/env/profile-local-vps-only.env)'
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

echo "local-vps-only: stopping local scraper (if running)"
docker stop streamclone-scraper 2>/dev/null || true
docker rm -f streamclone-scraper 2>/dev/null || true

echo "local-vps-only: done — scrape/bronze/tier-0 on remote worker only"
