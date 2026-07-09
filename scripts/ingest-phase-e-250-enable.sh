#!/usr/bin/env bash
# Scale to 500/250 — wide roster + max proven IRC (Phase D soak footprint).
# Tiering OFF: top 250 live channels get full IRC (simple, best coverage/efficiency tradeoff).
# MUST use production-up.sh --no-deps analytics (never raw docker compose up).
# Prerequisite: INGEST_CORE_ENABLED=1, dual/shadow off, Phase D soak PASS (250 IRC stable).
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

resolve_env_file() {
  if [[ -n "${ENV_FILE:-}" ]]; then
    echo "${ENV_FILE}"
    return 0
  fi
  local prod_local="${OPS_ROOT}/env/production.${STREAMPULSE_PROD_ENV_SUFFIX:-local.env}"
  if [[ -f "${prod_local}" ]]; then
    echo "${prod_local}"
    return 0
  fi
  if [[ -f "${OPS_ROOT}/env/production.env" ]]; then
    echo "${OPS_ROOT}/env/production.env"
    return 0
  fi
  echo "FAIL: no production env overlay under ${OPS_ROOT}/env — set ENV_FILE" >&2
  return 1
}

ENV_FILE="$(resolve_env_file)"
PRODUCTION_UP="${OPS_ROOT}/scripts/deploy/production-up.sh"

: "${IMAGE_TAG:?Set IMAGE_TAG to current production tag (e.g. v0.3.0-rc24)}"

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "FAIL: ${ENV_FILE} missing" >&2
  exit 1
fi

BACKUP="${ENV_FILE}.bak-before-phase-e-250-$(date -u +%Y%m%dT%H%M%SZ)"
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

# Core writer stays on; tiering off for flat top-N IRC (no P1 hot cap)
merge_key "INGEST_CORE_ENABLED" "1"
merge_key "INGEST_CORE_DUAL_READ_MODE" "0"
merge_key "INGEST_CORE_SHADOW_MODE" "0"
merge_key "INGEST_TIERING_ENABLED" "0"

merge_key "HUB_ROSTER_LIMIT" "500"
merge_key "INGEST_CANDIDATE_SCAN_TOP_N" "500"
merge_key "PUBLIC_HUB_LIVE_CAP" "250"
merge_key "MAX_ACTIVE_IRC_CHANNELS" "250"
merge_key "PULSE_MAX_ACTIVE_CHANNELS" "250"
merge_key "PULSE_LIVE_ADMISSION_ENABLED" "true"
merge_key "PULSE_LIVE_ADMISSION_TOP_N" "500"

# Tier-0 metadata for 500-channel hub awareness (IRC still capped at 250)
merge_key "TIER0_ENABLED" "1"
merge_key "TIER0_ROSTER_TOP_N" "500"

# Isolation unchanged
merge_key "CORPUS_WORKERS_ENABLED" "0"
merge_key "SCRAPER_ENABLED_ON_API_NODE" "0"

if grep -qE '^INGEST_SHADOW_CHANNEL_ALLOWLIST=' "${ENV_FILE}"; then
  sed -i '/^INGEST_SHADOW_CHANNEL_ALLOWLIST=/d' "${ENV_FILE}"
fi

echo "==> Phase E 500/250 env merged into ${ENV_FILE}"
grep -E '^(INGEST_CORE_|INGEST_TIERING|HUB_ROSTER|MAX_ACTIVE|PULSE_MAX|PULSE_LIVE|TIER0_)' "${ENV_FILE}" || true

if ! grep -qE '^INGEST_CORE_ENABLED=1' "${ENV_FILE}"; then
  echo "FAIL: INGEST_CORE_ENABLED must stay 1" >&2
  exit 1
fi
if grep -qE '^INGEST_CORE_DUAL_READ_MODE=1' "${ENV_FILE}" || grep -qE '^INGEST_CORE_SHADOW_MODE=1' "${ENV_FILE}"; then
  echo "FAIL: dual/shadow must be off" >&2
  exit 1
fi
if grep -qE '^INGEST_TIERING_ENABLED=1' "${ENV_FILE}"; then
  echo "FAIL: INGEST_TIERING_ENABLED must be 0 for 500/250 flat IRC" >&2
  exit 1
fi

echo "==> Restart analytics only (limits overlay included)"
ENV_FILE="${ENV_FILE}" IMAGE_TAG="${IMAGE_TAG}" bash "${PRODUCTION_UP}" --no-deps analytics

echo "==> Verify 500/250 env on analytics container"
ANALYTICS="$(docker ps --format '{{.Names}}' 2>/dev/null | grep -E 'analytics' | grep -v workers | head -1 || true)"
if [[ -n "${ANALYTICS}" ]]; then
  docker inspect "${ANALYTICS}" --format '{{range .Config.Env}}{{println .}}{{end}}' \
    | grep -E '^(INGEST_|HUB_ROSTER|MAX_ACTIVE|PULSE_MAX|PULSE_LIVE|TIER0_)' || true
fi

echo "DONE: 500/250 enabled — run hosted-limits-guard.sh then hub probes"
echo "Expect hub: tieringEnabled=false, activeCollectors ~249/250, trackingMax=250"
echo "Expect RAM ~400-500 MiB (Phase D baseline). Rollback to 500/50: ingest-phase-e-enable.sh"
