#!/usr/bin/env bash
# Self-heal BearHost Grafana SSH tunnel when localhost health check fails.
# Safe to run from cron or Windows Task Scheduler every few minutes.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-grafana-port.sh
source "${ROOT}/scripts/lib/bearhost-grafana-port.sh"

LOCAL_PORT="$(bearhost_grafana_local_port)"
LOG_FILE="${BEARHOST_GRAFANA_WATCH_LOG:-${HOME}/.streamclone/grafana-tunnel-watch.log}"
CURL_TIMEOUT_SEC="${BEARHOST_GRAFANA_WATCH_TIMEOUT_SEC:-5}"

watch_log() {
  mkdir -p "$(dirname "${LOG_FILE}")"
  printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >>"${LOG_FILE}"
}

if curl -sf -m "${CURL_TIMEOUT_SEC}" "http://127.0.0.1:${LOCAL_PORT}/api/health" >/dev/null 2>&1; then
  exit 0
fi

watch_log "health check failed on :${LOCAL_PORT}; restarting tunnel"
bash "${ROOT}/scripts/bearhost-grafana-tunnel-stop.sh" --quiet

if bash "${ROOT}/scripts/bearhost-grafana-tunnel-start.sh" >>"${LOG_FILE}" 2>&1; then
  watch_log "tunnel restored on :${LOCAL_PORT}"
  exit 0
fi

watch_log "tunnel restart failed — see log above; try: make grafana-setup"
exit 1
