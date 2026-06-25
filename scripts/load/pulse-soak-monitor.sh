#!/usr/bin/env bash
# Periodic Prometheus snapshots during 24h soak (append-only evidence).
set -euo pipefail

PROM="${PROMETHEUS_URL:-http://127.0.0.1:9090}"
EVIDENCE="${EVIDENCE:-docs/pulse-extension/soak-24h-evidence.txt}"
INTERVAL="${INTERVAL_SEC:-900}"
DURATION="${DURATION_SEC:-86400}"

query() {
  local q="$1"
  curl -sfG "${PROM}/api/v1/query" --data-urlencode "query=${q}" \
    | python3 -c "import json,sys; d=json.load(sys.stdin); r=d.get('data',{}).get('result',[]); print(r[0]['value'][1] if r else 'n/a')" 2>/dev/null \
    || echo "n/a"
}

end=$((SECONDS + DURATION))
echo "==> pulse-soak-monitor start $(date -u +%Y-%m-%dT%H:%M:%SZ)" | tee -a "${EVIDENCE}"
echo "prometheus=${PROM} interval=${INTERVAL}s duration=${DURATION}s" >> "${EVIDENCE}"

while (( SECONDS < end )); do
  ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  active="$(query 'pulse_active_tracked_channels')"
  backfill="$(query 'pulse_backfill_active_jobs')"
  up="$(query 'up{job="analytics"}')"
  line="${ts} active=${active} backfill=${backfill} up_analytics=${up}"
  echo "${line}" | tee -a "${EVIDENCE}"
  sleep "${INTERVAL}"
done

echo "==> pulse-soak-monitor end $(date -u +%Y-%m-%dT%H:%M:%SZ)" | tee -a "${EVIDENCE}"
