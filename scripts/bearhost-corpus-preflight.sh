#!/usr/bin/env bash
# BearHost corpus-plane preflight — Azure secret file + Twitch OAuth client creds.
# Source from deploy/smoke scripts; do not commit secrets.
#
# Usage:
#   source scripts/bearhost-corpus-preflight.sh
#   bearhost_corpus_preflight && echo ok
#
# Exit 0 when Azure secret file and TWITCH_CLIENT_ID/SECRET are present on the host.

bearhost_host_azure_secret_path() {
  local container_path="${ARCHIVE_AZURE_CONNECTION_STRING_FILE:-/run/streamclone-secrets/azure-archive-connection-string}"
  local basename
  basename="$(basename "${container_path}")"
  local secrets_dir="${STREAMCLONE_SECRETS_DIR:-/etc/streamclone/secrets}"
  printf '%s/%s' "${secrets_dir}" "${basename}"
}

bearhost_env_value() {
  local key="$1"
  local file="${2:-.env}"
  if [[ ! -f "${file}" ]]; then
    return 0
  fi
  grep -E "^${key}=" "${file}" 2>/dev/null | tail -n1 | cut -d= -f2- | tr -d '\r' || true
}

bearhost_twitch_credential_summary() {
  local file="${1:-.env}"
  local client_id client_secret oauth_id oauth_secret
  client_id="$(bearhost_env_value TWITCH_CLIENT_ID "${file}")"
  client_secret="$(bearhost_env_value TWITCH_CLIENT_SECRET "${file}")"
  oauth_id="$(bearhost_env_value TWITCH_OAUTH_CLIENT_ID "${file}")"
  oauth_secret="$(bearhost_env_value TWITCH_OAUTH_CLIENT_SECRET "${file}")"

  if [[ -n "${oauth_id}" && -n "${oauth_secret}" ]]; then
    printf 'TWITCH_OAUTH_CLIENT_ID/SECRET'
    return 0
  fi
  if [[ -n "${client_id}" && -n "${client_secret}" ]]; then
    printf 'TWITCH_CLIENT_ID/SECRET'
    return 0
  fi
  return 1
}

bearhost_corpus_preflight() {
  local secret_path failures=()
  secret_path="$(bearhost_host_azure_secret_path)"
  if [[ ! -f "${secret_path}" ]]; then
    failures+=("Azure secret file missing: ${secret_path}")
  fi
  local twitch_summary=""
  if ! twitch_summary="$(bearhost_twitch_credential_summary ".env")"; then
    failures+=("No complete Twitch credential pair in .env (need TWITCH_CLIENT_ID/SECRET or TWITCH_OAUTH_CLIENT_ID/SECRET)")
  fi
  if ((${#failures[@]} > 0)); then
    echo "bearhost-corpus-preflight FAIL:" >&2
    for msg in "${failures[@]}"; do
      echo "  - ${msg}" >&2
    done
    return 1
  fi
  echo "bearhost-corpus-preflight PASS: ${secret_path}; ${twitch_summary} present"
  return 0
}
