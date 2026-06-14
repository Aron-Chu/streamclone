#!/usr/bin/env bash
# Fallback: restart port-forwards when LoadBalancer is unavailable (non–Docker Desktop).
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
namespace="${HELM_NAMESPACE:-streamclone}"
echo "Pulse port-forward watchdog (Ctrl+C to stop). LoadBalancer preferred when localhost responds."
while true; do
  HELM_NAMESPACE="$namespace" bash scripts/helm-portforward.sh start all || true
  sleep 30
done
