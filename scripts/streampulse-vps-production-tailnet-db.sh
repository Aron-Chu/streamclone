#!/usr/bin/env bash
# Expose streampulse-vps production Postgres/Redis on the tailnet IP for BearHost remote workers.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/streampulse-vps-production-compose.sh
source "${ROOT}/scripts/lib/streampulse-vps-production-compose.sh"

WORKER="${WORKER:-23.173.152.156}"
WORKER_APP="${WORKER_APP:-/opt/streamclone/app}"
streampulse_vps_resolve_worker_key
ssh_worker() { ssh -i "${WORKER_KEY}" -o BatchMode=yes "root@${WORKER}" "$@"; }

echo "==> enable tailnet DB bind on streampulse-vps"
ssh_worker bash -s <<REMOTE
set -euo pipefail
cd ${WORKER_APP}
BIND_IP="\$(tailscale ip -4 2>/dev/null || true)"
if [[ -z "\${BIND_IP}" ]]; then
  echo "ERROR: tailscale ip -4 empty" >&2
  exit 1
fi
export STREAMPULSE_TAILNET_BIND_IP="\${BIND_IP}"
# shellcheck source=scripts/lib/streampulse-vps-production-compose.sh
source scripts/lib/streampulse-vps-production-compose.sh
streampulse_vps_production_compose "\$(pwd)" \
  -f deploy/docker-compose.streampulse-vps-production-tailnet.yml \
  up -d postgres redis
echo "tailnet bind \${BIND_IP}:5432 and \${BIND_IP}:6379"
ss -lntp | grep -E ':5432|:6379' || true
REMOTE

echo "done"
