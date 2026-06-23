#!/usr/bin/env bash
# SSH tunnel to BearHost Grafana (Archive Corpus dashboard).
# Usage: bash scripts/bearhost-grafana-tunnel.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"
# shellcheck source=scripts/lib/bearhost-grafana-port.sh
source "${ROOT}/scripts/lib/bearhost-grafana-port.sh"

bearhost_ssh_config
LOCAL_PORT="$(bearhost_grafana_local_port)"
DASH_URL="$(bearhost_grafana_dashboard_url)"

bearhost_grafana_warn_local_pulse

echo "==> Grafana tunnel: http://localhost:${LOCAL_PORT} (Ctrl+C to stop)"
echo "    Dashboard: ${DASH_URL}"
echo "    Login: admin / streampulse"
exec ssh -i "${BEARHOST_SSH_KEY}" \
  -o ServerAliveInterval=30 \
  -o ServerAliveCountMax=6 \
  -o TCPKeepAlive=yes \
  -N -L "${LOCAL_PORT}:127.0.0.1:3000" \
  "${BEARHOST_USER}@${BEARHOST_HOST}"
