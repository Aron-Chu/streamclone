#!/usr/bin/env bash
# CAP-001 — raise production cap to 25 after soak PASS + operator sign-off.
# Requires: CAP001_APPROVED=1 and soak evidence marked PASS.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "${ROOT}"

EVIDENCE="${ROOT}/docs/pulse-extension/soak-24h-evidence.txt"
CAP_EVIDENCE="${ROOT}/docs/pulse-extension/cap-001-evidence.txt"
ENV_FILE="${ROOT}/deploy/env/profile-bearhost-pulse.env"

if [[ "${CAP001_APPROVED:-}" != "1" ]]; then
  echo "FAIL: set CAP001_APPROVED=1 after operator sign-off in cap-001-evidence.txt" >&2
  exit 2
fi

if grep -q 'IN PROGRESS' "${CAP_EVIDENCE}" 2>/dev/null; then
  echo "FAIL: 24h soak not marked PASS in ${CAP_EVIDENCE}" >&2
  exit 2
fi
if ! grep -q 'PASS' "${EVIDENCE}" 2>/dev/null; then
  echo "FAIL: soak evidence not marked PASS in ${EVIDENCE}" >&2
  exit 2
fi

if tmux has-session -t pulse-soak 2>/dev/null; then
  echo "FAIL: tmux pulse-soak still running — wait for soak to finish" >&2
  exit 2
fi

echo "==> CAP-001: raise production cap to 25"
grep '^PULSE_MAX_ACTIVE_CHANNELS=' "${ENV_FILE}"
grep '^MAX_CONCURRENT_TRACKED_CHANNELS=' "${ENV_FILE}"

sed -i 's/^PULSE_MAX_ACTIVE_CHANNELS=.*/PULSE_MAX_ACTIVE_CHANNELS=25/' "${ENV_FILE}"
sed -i 's/^MAX_CONCURRENT_TRACKED_CHANNELS=.*/MAX_CONCURRENT_TRACKED_CHANNELS=25/' "${ENV_FILE}"

echo "==> redeploy (run from dev machine with SSH):"
echo "  bash scripts/bearhost-rsync-to-vps.sh"
echo "  bash scripts/bearhost-pulse-redeploy-remote.sh"
echo "  PULSE_SMOKE_BASE_URL=https://api.streampulse.stream PULSE_EXPECT_HOSTED_MODE=true bash deploy/smoke/bearhost-pulse-api.sh"
echo ""
echo "Updated ${ENV_FILE} locally — commit + rsync + redeploy to apply."
