#!/usr/bin/env bash
# Start BearHost Grafana SSH tunnel in background on local port 3001.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
source "${ROOT}/scripts/lib/bearhost-ssh.sh"
source "${ROOT}/scripts/lib/bearhost-grafana-port.sh"

bash "${ROOT}/scripts/bearhost-grafana-tunnel-stop.sh"

bearhost_ssh_config
LOCAL_PORT="$(bearhost_grafana_local_port)"
ssh -f -i "${BEARHOST_SSH_KEY}" \
  -o ExitOnForwardFailure=yes \
  -o ServerAliveInterval=60 \
  -N -L "${LOCAL_PORT}:127.0.0.1:3000" \
  "${BEARHOST_USER}@${BEARHOST_HOST}"
sleep 1

if ! curl -sf "http://127.0.0.1:${LOCAL_PORT}/api/health" >/dev/null; then
  echo "bearhost-grafana-tunnel-start: tunnel up but Grafana health check failed on :${LOCAL_PORT}" >&2
  exit 1
fi

echo "==> Grafana tunnel running on http://localhost:${LOCAL_PORT}"
echo "    Dashboard: http://localhost:${LOCAL_PORT}/d/streamclone-archive/streamclone-archive"
echo "    Login: admin / streampulse"
echo "    Stop: make bearhost-grafana-tunnel-stop"
