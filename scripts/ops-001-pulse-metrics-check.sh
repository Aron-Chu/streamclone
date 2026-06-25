#!/usr/bin/env bash
# OPS-001 — query Prometheus for Pulse capacity metrics (local or BearHost tunnel).
set -euo pipefail

PROM="${PROMETHEUS_URL:-http://127.0.0.1:9090}"
TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

query() {
  local q="$1"
  local enc
  enc="$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$q")"
  curl -sf "${PROM}/api/v1/query?query=${enc}" 2>/dev/null || echo '{"status":"error"}'
}

echo "# OPS-001 pulse metrics check — ${TS}"
echo "# PROMETHEUS_URL=${PROM}"
echo ""

METRICS=(
  'max(pulse_active_tracked_channels{job="analytics"})'
  'max(pulse_backfill_active_jobs{job="analytics"})'
  'max(up{job="analytics"})'
  'sum(pulse_golive_detected_total{job="analytics"})'
  'sum(pulse_tracked_from_start_total{job="analytics"})'
  'sum(rate(http_requests_total{job="analytics",status=~"5.."}[5m]))'
)

for q in "${METRICS[@]}"; do
  echo "## ${q}"
  query "$q" | python3 -m json.tool 2>/dev/null || query "$q"
  echo ""
done

echo "## pulse_golive_to_first_rollup_seconds (histogram present?)"
curl -sf "${PROM}/api/v1/label/__name__/values" 2>/dev/null \
  | python3 -c "import json,sys; v=json.load(sys.stdin).get('data',[]); print('present' if any('pulse_golive_to_first_rollup_seconds' in x for x in v) else 'missing')" \
  || echo "could not list metric names"
