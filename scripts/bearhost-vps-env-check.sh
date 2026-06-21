#!/usr/bin/env bash
# BearHost VPS env/secret check — validates the host-side secret and required .env keys.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

# shellcheck source=scripts/bearhost-corpus-preflight.sh
source "${ROOT}/scripts/bearhost-corpus-preflight.sh"

ENV_FILE="${BEARHOST_ENV_FILE:-.env}"
failures=()

secret_path="$(bearhost_host_azure_secret_path)"
if [[ ! -f "${secret_path}" ]]; then
  failures+=("Install Azure archive secret at ${secret_path}")
fi

scraper_api_key="$(bearhost_env_value SCRAPER_API_KEY "${ENV_FILE}")"
if [[ -z "${scraper_api_key}" ]]; then
  failures+=("Add SCRAPER_API_KEY to ${ENV_FILE}")
fi

twitch_summary=""
if ! twitch_summary="$(bearhost_twitch_credential_summary "${ENV_FILE}")"; then
  failures+=("Add TWITCH_CLIENT_ID/SECRET or TWITCH_OAUTH_CLIENT_ID/SECRET to ${ENV_FILE}")
fi

if ((${#failures[@]} > 0)); then
  echo "bearhost-vps-env-check FAIL:" >&2
  for msg in "${failures[@]}"; do
    echo "  - ${msg}" >&2
  done
  exit 2
fi

echo "bearhost-vps-env-check PASS: ${secret_path}; SCRAPER_API_KEY present; ${twitch_summary} present"
