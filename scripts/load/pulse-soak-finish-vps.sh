#!/usr/bin/env bash
# Batch R3 Phase 2 — post-soak evidence + smokes (run after tmux pulse-soak completes).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "${ROOT}"

EVIDENCE="${EVIDENCE:-docs/pulse-extension/soak-24h-evidence.txt}"
CAP_EVIDENCE="${CAP_EVIDENCE:-docs/pulse-extension/cap-001-evidence.txt}"
SESSION="${PULSE_SOAK_TMUX_SESSION:-pulse-soak}"

echo "==> Batch R3 post-soak finish"

if tmux has-session -t "${SESSION}" 2>/dev/null; then
  echo "WARN: tmux session ${SESSION} still running — soak not finished yet" >&2
  echo "Attach: tmux attach -t ${SESSION}"
  exit 2
fi

END_UTC="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "==> soak end ${END_UTC}" | tee -a "${EVIDENCE}"

echo "==> docker stats sample"
docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}" \
  streamclone-analytics-1 2>/dev/null | tee -a "${EVIDENCE}" || true

echo "==> hosted smokes"
PULSE_SMOKE_BASE_URL="${PULSE_SMOKE_BASE_URL:-https://api.streampulse.stream}" \
  PULSE_EXPECT_HOSTED_MODE=true \
  bash deploy/smoke/bearhost-pulse-api.sh

bash deploy/smoke/test-013b-hosted.sh

echo ""
echo "Next: mark soak PASS in ${EVIDENCE}, update ${CAP_EVIDENCE}, operator sign-off, then CAP-001 raise if approved."
