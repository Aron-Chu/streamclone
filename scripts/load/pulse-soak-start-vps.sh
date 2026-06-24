#!/usr/bin/env bash
# Batch R3 — start 24h production soak monitor in tmux on BearHost (PC can detach).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "${ROOT}"

SESSION="${PULSE_SOAK_TMUX_SESSION:-pulse-soak}"
EVIDENCE="${EVIDENCE:-docs/pulse-extension/soak-24h-evidence.txt}"
PROM="${PROMETHEUS_URL:-http://127.0.0.1:9090}"
INTERVAL="${INTERVAL_SEC:-900}"
DURATION="${DURATION_SEC:-86400}"

if tmux has-session -t "${SESSION}" 2>/dev/null; then
  echo "FAIL: tmux session ${SESSION} already exists — attach with: tmux attach -t ${SESSION}"
  exit 2
fi

START_UTC="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
{
  echo ""
  echo "==> soak monitor start ${START_UTC}"
  echo "target=http://127.0.0.1:8090 production cap=10"
  echo "prometheus=${PROM} interval=${INTERVAL}s duration=${DURATION}s"
} | tee -a "${EVIDENCE}"

tmux new-session -d -s "${SESSION}" \
  "cd '${ROOT}' && export EVIDENCE='${EVIDENCE}' PROMETHEUS_URL='${PROM}' INTERVAL_SEC='${INTERVAL}' DURATION_SEC='${DURATION}' && bash scripts/load/pulse-soak-monitor.sh; echo '==> soak monitor finished' $(date -u +%Y-%m-%dT%H:%M:%SZ) | tee -a '${EVIDENCE}'"

echo "PASS: started tmux session ${SESSION}"
echo "Attach: tmux attach -t ${SESSION}"
echo "Detach: Ctrl-b d"
echo "Evidence: ${EVIDENCE}"
