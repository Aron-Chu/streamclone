#!/usr/bin/env bash
# OPS-001 VPS checks — Prometheus Pulse metrics + Grafana dashboard provisioned.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PROM="${PROMETHEUS_URL:-http://127.0.0.1:9090}"

echo "==> OPS-001 VPS checks"

echo "--- observability containers"
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" \
  | grep -E "grafana-obs|prometheus-obs|NAMES" || true

echo "--- cap (profile, no raise)"
grep -E "PULSE_MAX_ACTIVE_CHANNELS|PULSE_MAX_BACKFILLS" \
  "${ROOT}/deploy/env/profile-bearhost-pulse.env" || true
docker exec streamclone-analytics-1 printenv PULSE_MAX_ACTIVE_CHANNELS 2>/dev/null \
  || echo "PULSE_MAX_ACTIVE_CHANNELS=(unset in container)"

echo "--- Prometheus Pulse metrics"
PROMETHEUS_URL="${PROM}" bash "${ROOT}/scripts/ops-001-pulse-metrics-check.sh"

echo "--- Grafana dashboard uid streamclone-pulse-capacity"
if [[ -n "${GRAFANA_ADMIN_PASSWORD:-}" ]]; then
  if curl -sf -u "${GRAFANA_ADMIN_USER:-admin}:${GRAFANA_ADMIN_PASSWORD}" \
    "http://127.0.0.1:3000/api/dashboards/uid/streamclone-pulse-capacity" \
    | python3 -c "import json,sys; d=json.load(sys.stdin).get('dashboard',{}); print('title:', d.get('title')); print('panels:', len(d.get('panels',[])))"; then
    echo "PASS: dashboard provisioned"
  else
    echo "WARN: dashboard API check failed (tunnel not required for localhost on VPS)"
  fi
else
  echo "SKIP: set GRAFANA_ADMIN_PASSWORD to verify dashboard API"
fi

echo "OPS-001 VPS: checks complete"
