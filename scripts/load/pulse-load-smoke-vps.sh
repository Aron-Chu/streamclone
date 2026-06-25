#!/usr/bin/env bash
# LOAD-001 smoke on BearHost localhost — reads beta key from VPS secrets (never prints key).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SECRET="/etc/streamclone/secrets/pulse-beta.env"
EVIDENCE="${ROOT}/docs/pulse-extension/load-001-smoke-evidence.txt"

echo "==> LOAD-001 smoke (localhost, cap-safe)"

if [[ -f "${SECRET}" ]]; then
  # shellcheck disable=SC1090
  source "${SECRET}"
fi

if [[ -z "${PULSE_BETA_KEYS:-}" && -z "${PULSE_LOAD_BETA_KEY:-}" ]]; then
  echo "WARN: no PULSE_BETA_KEYS in ${SECRET} — reading from analytics container inspect"
  key="$(
    docker inspect streamclone-analytics-1 --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null \
      | sed -n 's/^PULSE_BETA_KEYS=//p' | cut -d, -f1
  )"
  if [[ -n "${key}" ]]; then
    export PULSE_LOAD_BETA_KEY="${key}"
  fi
elif [[ -z "${PULSE_LOAD_BETA_KEY:-}" && -n "${PULSE_BETA_KEYS:-}" ]]; then
  export PULSE_LOAD_BETA_KEY="${PULSE_BETA_KEYS%%,*}"
fi

if [[ -z "${PULSE_LOAD_BETA_KEY:-}" ]]; then
  echo "FAIL: beta key not available (set ${SECRET} or PULSE_LOAD_BETA_KEY)" >&2
  exit 2
fi

export PULSE_LOAD_TARGET="${PULSE_LOAD_TARGET:-http://127.0.0.1:8090}"
export PULSE_LOAD_MODE=smoke
export PULSE_LOAD_CHANNEL_COUNT="${PULSE_LOAD_CHANNEL_COUNT:-3}"
export PULSE_LOAD_STAGGER_MS="${PULSE_LOAD_STAGGER_MS:-2000}"
export PULSE_LOAD_PROMETHEUS_URL="${PULSE_LOAD_PROMETHEUS_URL:-http://127.0.0.1:9090}"
export PULSE_LOAD_EVIDENCE_FILE="${EVIDENCE}"
export PULSE_LOAD_PRODUCTION_CAP=10

echo "target=${PULSE_LOAD_TARGET} channels=${PULSE_LOAD_CHANNEL_COUNT} prometheus=${PULSE_LOAD_PROMETHEUS_URL}"

bash "${ROOT}/scripts/load/pulse-25-channel-harness.sh"

echo ""
echo "==> docker stats checkpoint (analytics, one sample)"
docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}" \
  streamclone-analytics-1 2>/dev/null || echo "docker stats unavailable"

echo "Evidence: ${EVIDENCE}"
