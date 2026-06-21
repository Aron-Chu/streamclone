#!/usr/bin/env bash
# Rsync Grafana/Prometheus configs to VPS and reload observability stack.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"

bash "${ROOT}/scripts/bearhost-rsync-to-vps.sh"

bearhost_ssh_config
echo "==> reload observability on ${BEARHOST_USER}@${BEARHOST_HOST}"
bearhost_ssh_script bearhost-observability.sh up

echo ""
echo "Tunnel: make bearhost-grafana-tunnel"
echo "Open:   http://localhost:3001/d/streamclone-archive/streamclone-archive"
