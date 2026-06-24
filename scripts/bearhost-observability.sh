#!/usr/bin/env bash
# Optional Prometheus + Grafana on BearHost (localhost bind only — use SSH tunnel).
# Usage: scripts/bearhost-observability.sh up|down|status|logs
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

# shellcheck source=scripts/lib/bearhost-compose.sh
source "${ROOT}/scripts/lib/bearhost-compose.sh"

ACTION="${1:-status}"
shift || true

case "${ACTION}" in
  up)
    echo "bearhost-observability: starting Prometheus + Grafana + node-exporter (profile observability)"
    bearhost_compose_obs up -d prometheus-obs node-exporter-obs grafana-obs
    echo ""
    echo "bearhost-observability: Grafana (SSH tunnel required)"
    echo "  dashboard: http://localhost:3001/d/streamclone-archive/streamclone-archive"
    echo "  login:     admin / streampulse"
    echo "  tunnel:    make grafana-up"
    bearhost_compose_obs ps prometheus-obs grafana-obs
    ;;
  down)
    echo "bearhost-observability: stopping observability profile"
    bearhost_compose_obs stop prometheus-obs grafana-obs 2>/dev/null || true
    bearhost_compose_obs rm -f prometheus-obs grafana-obs 2>/dev/null || true
    ;;
  status|ps)
    bearhost_compose_obs ps prometheus-obs grafana-obs 2>/dev/null || bearhost_compose_obs ps
    ;;
  logs)
    bearhost_compose_obs logs -f --tail=100 prometheus-obs grafana-obs "$@"
    ;;
  *)
    echo "Usage: scripts/bearhost-observability.sh up|down|status|logs" >&2
    exit 2
    ;;
esac
