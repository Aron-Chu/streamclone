#!/usr/bin/env bash
# Start BearHost Grafana SSH tunnel in background on local port 3001.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
source "${ROOT}/scripts/lib/bearhost-ssh.sh"
source "${ROOT}/scripts/lib/bearhost-grafana-port.sh"

bash "${ROOT}/scripts/bearhost-grafana-tunnel-stop.sh" --quiet

bearhost_ssh_config
LOCAL_PORT="$(bearhost_grafana_local_port)"
echo "==> Starting Grafana SSH tunnel → http://localhost:${LOCAL_PORT}"

ssh -f -i "${BEARHOST_SSH_KEY}" \
  -o ExitOnForwardFailure=yes \
  -o ServerAliveInterval=30 \
  -o ServerAliveCountMax=6 \
  -o TCPKeepAlive=yes \
  -N -L "${LOCAL_PORT}:127.0.0.1:3000" \
  "${BEARHOST_USER}@${BEARHOST_HOST}"

health_ok=0
for _ in 1 2 3 4 5; do
  if curl -sf "http://127.0.0.1:${LOCAL_PORT}/api/health" >/dev/null; then
    health_ok=1
    break
  fi
  sleep 2
done

if [[ "$health_ok" -ne 1 ]]; then
  echo "bearhost-grafana-tunnel-start: tunnel is up but Grafana on the VPS is not responding on :${LOCAL_PORT}" >&2
  echo "  Try: make grafana-setup   (first-time — start prometheus-obs + grafana-obs on VPS)" >&2
  echo "  Or:  make bearhost-observability-status" >&2
  exit 1
fi

echo "==> Grafana tunnel running on http://localhost:${LOCAL_PORT}"
echo "    Dashboard: http://localhost:${LOCAL_PORT}/d/streamclone-archive/streamclone-archive"
echo "    Login: admin / streampulse"
echo "    Stop: make grafana-stop"
