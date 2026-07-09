#!/usr/bin/env bash
# Tiering-on 500 scan / 250 IRC — aggressive live chat coverage (post-audit plan).
# Scans HUB_ROSTER_LIMIT=500 live candidates; P1 hot ranks 1..250 consume IRC slots.
# Do NOT use ingest-phase-e-250-enable.sh for this profile (that script sets tiering off).
#
# MUST use production-up.sh --no-deps analytics (never raw docker compose up).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

resolve_ops_root() {
  local candidate
  for candidate in \
    "${STREAMPULSE_OPS_ROOT:-}" \
    "${HOME}/streampulse-ops"; do
    if [[ -n "${candidate}" && -f "${candidate}/scripts/deploy/production-up.sh" ]]; then
      echo "${candidate}"
      return 0
    fi
  done
  echo "FAIL: private streampulse-ops checkout not found — set STREAMPULSE_OPS_ROOT" >&2
  return 1
}

OPS_ROOT="$(resolve_ops_root)"
ENV_FILE="${OPS_ROOT}/env/production.env"
PRODUCTION_UP="${OPS_ROOT}/scripts/deploy/production-up.sh"

: "${IMAGE_TAG:?Set IMAGE_TAG to current production tag (e.g. v0.3.0-rc25)}"

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "FAIL: ${ENV_FILE} missing" >&2
  exit 1
fi

BACKUP="${ENV_FILE}.bak-before-tiering-500-250-$(date -u +%Y%m%dT%H%M%SZ)"
cp -a "${ENV_FILE}" "${BACKUP}"
echo "Backed up env to ${BACKUP}"

merge_key() {
  local key="$1"
  local value="$2"
  if grep -qE "^${key}=" "${ENV_FILE}"; then
    sed -i "s|^${key}=.*|${key}=${value}|" "${ENV_FILE}"
  else
    echo "${key}=${value}" >> "${ENV_FILE}"
  fi
}

merge_key "INGEST_CORE_ENABLED" "1"
merge_key "INGEST_CORE_DUAL_READ_MODE" "0"
merge_key "INGEST_CORE_SHADOW_MODE" "0"
merge_key "INGEST_TIERING_ENABLED" "1"
merge_key "HUB_ROSTER_LIMIT" "500"
merge_key "INGEST_P1_HOT_LIMIT" "250"
merge_key "INGEST_CANDIDATE_SCAN_TOP_N" "500"
merge_key "MAX_ACTIVE_IRC_CHANNELS" "250"
merge_key "PULSE_MAX_ACTIVE_CHANNELS" "250"
merge_key "PUBLIC_HUB_LIVE_CAP" "250"
merge_key "PULSE_LIVE_ADMISSION_ENABLED" "true"
merge_key "PULSE_LIVE_ADMISSION_SOURCE" "roster_then_helix"
merge_key "PULSE_LIVE_ADMISSION_INTERVAL" "15s"

merge_key "CORPUS_WORKERS_ENABLED" "0"
merge_key "SCRAPER_ENABLED_ON_API_NODE" "0"

echo "==> Tiering-on 500/250 env merged into ${ENV_FILE}"
grep -E '^(INGEST_|HUB_ROSTER|MAX_ACTIVE|PULSE_MAX|PULSE_LIVE|PUBLIC_HUB)' "${ENV_FILE}" || true

if ! grep -qE '^INGEST_CORE_ENABLED=1' "${ENV_FILE}"; then
  echo "FAIL: INGEST_CORE_ENABLED must stay 1" >&2
  exit 1
fi
if ! grep -qE '^INGEST_TIERING_ENABLED=1' "${ENV_FILE}"; then
  echo "FAIL: INGEST_TIERING_ENABLED must be 1 for 500 scan / 250 P1 hot" >&2
  exit 1
fi

echo "==> Restart analytics only"
ENV_FILE="${ENV_FILE}" IMAGE_TAG="${IMAGE_TAG}" bash "${PRODUCTION_UP}" --no-deps analytics

echo "==> Verify env on analytics container"
ANALYTICS="$(docker ps --format '{{.Names}}' 2>/dev/null | grep -E 'analytics' | grep -v workers | head -1 || true)"
if [[ -n "${ANALYTICS}" ]]; then
  docker inspect "${ANALYTICS}" --format '{{range .Config.Env}}{{println .}}{{end}}' \
    | grep -E '^(INGEST_|HUB_ROSTER|MAX_ACTIVE|PULSE_MAX|PULSE_LIVE|PUBLIC_HUB)' || true
fi

echo "DONE: tiering-on 500/250 enabled"
echo "Next: bash scripts/smoke/hosted-limits-guard.sh (from private streampulse-ops checkout)"
echo "Expect hub: tieringEnabled=true, activeCollectors ~249/250, poolSize up to PUBLIC_HUB_LIVE_CAP (250)"
