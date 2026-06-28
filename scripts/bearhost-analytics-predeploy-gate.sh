#!/usr/bin/env bash
# Read-only BearHost analytics deploy gate.
# Hard-blocks analytics container recreate when migration 000050 columns are missing.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"
# shellcheck source=scripts/lib/bearhost-analytics-gate-checks.sh
source "${ROOT}/scripts/lib/bearhost-analytics-gate-checks.sh"

bearhost_ssh_config

gate_mode="remote"
if [[ "${BEARHOST_ANALYTICS_GATE_REMOTE:-}" == "1" ]]; then
  gate_mode="remote"
elif [[ "${BEARHOST_ANALYTICS_GATE_LOCAL:-}" == "1" ]]; then
  gate_mode="local"
elif docker exec streamclone-postgres-1 psql -U app -d streamclone -P pager=off -c 'SELECT 1' >/dev/null 2>&1; then
  gate_mode="local"
fi

echo "==> BearHost analytics predeploy gate (read-only)"
echo "    mode=${gate_mode} host=${BEARHOST_USER}@${BEARHOST_HOST}"

if [[ "${gate_mode}" == "local" ]]; then
  bearhost_analytics_gate_checks
else
  bearhost_ssh "cd '${BEARHOST_REMOTE_APP}' && bash scripts/lib/bearhost-analytics-gate-checks-remote.sh"
fi
