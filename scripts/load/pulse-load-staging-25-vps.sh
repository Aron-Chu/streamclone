#!/usr/bin/env bash
# LOAD-001b — staging-25 on BearHost isolated stack (:8091).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SECRET="/etc/streamclone/secrets/pulse-beta.env"
EVIDENCE="${ROOT}/docs/pulse-extension/load-001b-staging-evidence.txt"

if [[ -f "${SECRET}" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "${SECRET}"
  set +a
fi

if [[ -z "${PULSE_LOAD_BETA_KEYS:-}" && -z "${PULSE_BETA_KEYS:-}" ]]; then
  echo "WARN: no PULSE_BETA_KEYS in ${SECRET} — reading from production analytics inspect"
  keys="$(
    docker inspect streamclone-analytics-1 --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null \
      | sed -n 's/^PULSE_BETA_KEYS=//p'
  )"
  if [[ -n "${keys}" ]]; then
    export PULSE_LOAD_BETA_KEYS="${keys}"
  fi
elif [[ -z "${PULSE_LOAD_BETA_KEYS:-}" && -n "${PULSE_BETA_KEYS:-}" ]]; then
  export PULSE_LOAD_BETA_KEYS="${PULSE_BETA_KEYS}"
fi

if [[ -z "${PULSE_LOAD_BETA_KEYS:-}" ]]; then
  echo "FAIL: beta keys not available (set ${SECRET} or PULSE_LOAD_BETA_KEYS)" >&2
  exit 2
fi

echo "==> staging stack up"
bash "${ROOT}/scripts/bearhost-pulse-staging-up.sh"

for i in 1 2 3 4 5; do
  if curl -sf "http://127.0.0.1:8091/v1/extension/health" >/dev/null 2>&1; then
    break
  fi
  echo "waiting for staging health (${i}/5)..."
  sleep 5
done

if ! curl -sf "http://127.0.0.1:8091/v1/extension/health" >/dev/null; then
  echo "FAIL: staging health not ready on :8091" >&2
  docker ps --filter name=streamclone-pulse-staging --format 'table {{.Names}}\t{{.Status}}' || true
  exit 2
fi

export PULSE_LOAD_TARGET="${PULSE_LOAD_TARGET:-http://127.0.0.1:8091}"
export PULSE_LOAD_MODE=staging-25
export PULSE_LOAD_STAGING_CONFIRM=1
export PULSE_LOAD_PROMETHEUS_URL="${PULSE_LOAD_PROMETHEUS_URL:-http://127.0.0.1:9090}"
export PULSE_LOAD_EVIDENCE_FILE="${EVIDENCE}"
export PULSE_LOAD_PRODUCTION_CAP=25

echo "target=${PULSE_LOAD_TARGET} mode=staging-25 prometheus=${PULSE_LOAD_PROMETHEUS_URL}"

bash "${ROOT}/scripts/load/pulse-25-channel-harness.sh"

echo ""
echo "==> docker stats checkpoint (staging analytics)"
docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}" \
  streamclone-pulse-staging-analytics 2>/dev/null || echo "docker stats unavailable"

echo "Evidence: ${EVIDENCE}"
