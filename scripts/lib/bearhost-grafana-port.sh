#!/usr/bin/env bash
# Shared BearHost Grafana tunnel port helpers.
set -euo pipefail

bearhost_grafana_local_port() {
  printf '%s\n' "${BEARHOST_GRAFANA_LOCAL_PORT:-3001}"
}

bearhost_grafana_dashboard_url() {
  local port
  port="$(bearhost_grafana_local_port)"
  printf 'http://localhost:%s/d/streamclone-archive/streamclone-archive\n' "$port"
}

bearhost_grafana_warn_local_pulse() {
  if [[ -z "${GRAFANA_ADMIN_USER:-}" || -z "${GRAFANA_ADMIN_PASSWORD:-}" ]]; then
    return 0
  fi
  local local_ds
  local_ds="$(curl -sf -u "${GRAFANA_ADMIN_USER}:${GRAFANA_ADMIN_PASSWORD}" \
    http://127.0.0.1:3000/api/datasources 2>/dev/null || true)"
  if echo "$local_ds" | grep -q '"type":"influxdb"'; then
    echo ""
    echo "NOTE: local Pulse Grafana is on http://localhost:3000 (InfluxDB — archive panels show no data)."
    echo "      VPS archive dashboard: $(bearhost_grafana_dashboard_url)"
  fi
}
