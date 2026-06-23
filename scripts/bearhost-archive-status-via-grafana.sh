#!/usr/bin/env bash
# Query scrape-archive metrics through the local BearHost Grafana SSH tunnel (:3001).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-grafana-port.sh
source "${ROOT}/scripts/lib/bearhost-grafana-port.sh"

PORT="$(bearhost_grafana_local_port)"
GRAFANA_USER="${GRAFANA_ADMIN_USER:-admin}"
GRAFANA_PASS="${GRAFANA_ADMIN_PASSWORD:-}"
if [[ -z "${GRAFANA_PASS}" && -f "${ROOT}/deploy/env/.grafana-admin-password" ]]; then
  GRAFANA_PASS="$(tr -d '\r\n' < "${ROOT}/deploy/env/.grafana-admin-password")"
fi
if [[ -z "${GRAFANA_PASS}" ]]; then
  echo "Set GRAFANA_ADMIN_PASSWORD or deploy/env/.grafana-admin-password" >&2
  exit 1
fi
AUTH="${GRAFANA_USER}:${GRAFANA_PASS}"
DS_UID="${GRAFANA_PROMETHEUS_UID:-prometheus}"

if ! curl -sf -m 5 -u "${AUTH}" "http://127.0.0.1:${PORT}/api/health" >/dev/null; then
  echo "Grafana not reachable on :${PORT}. Run: make grafana-up" >&2
  exit 1
fi

prom_query() {
  local q="$1"
  curl -sf -m 15 -u "${AUTH}" -G \
    "http://127.0.0.1:${PORT}/api/datasources/proxy/uid/${DS_UID}/api/v1/query" \
    --data-urlencode "query=${q}"
}

scalar() {
  prom_query "$1" | python3 -c "
import json,sys
r=json.load(sys.stdin).get('data',{}).get('result',[])
if not r:
    print('n/a')
else:
    print(r[0].get('value',['',''])[1])
"
}

echo "==> Streamclone archive (via Grafana tunnel :${PORT})"
echo "    Dashboard: http://localhost:${PORT}/d/streamclone-archive/streamclone-archive"
echo ""

worker_up="$(scalar 'max(up{job="analytics-workers"})')"
echo "analytics-workers up: ${worker_up}"

bronze_roster="$(scalar 'max(bronze_channels_roster_gauge{job="analytics-workers"})')"
bronze_target="$(scalar 'max(bronze_channels_target{job="analytics-workers"})')"
echo "bronze roster: ${bronze_roster} / ${bronze_target} channels"

silver_pct="$(scalar '100 * max(corpus_tier_completion_ratio{job="analytics-workers",tier="silver",measure="export_confirmed"})')"
gold_pct="$(scalar '100 * max(corpus_tier_completion_ratio{job="analytics-workers",tier="gold",measure="export_confirmed"})')"
echo "silver export confirmed: ${silver_pct}%"
echo "gold export confirmed: ${gold_pct}%"

echo ""
echo "backfill jobs (tier × status):"
prom_query 'sum by (tier, status) (backfill_jobs_gauge{job="analytics-workers"})' | python3 -c "
import json,sys
rows=json.load(sys.stdin).get('data',{}).get('result',[])
if not rows:
    print('  (no metrics — workers may be down or observability stale)')
for row in sorted(rows, key=lambda r: (r['metric'].get('tier',''), r['metric'].get('status',''))):
    m=row['metric']
    print(f\"  {m.get('tier','?'):6} {m.get('status','?'):8} {row['value'][1]}\")
"

stale="$(scalar 'sum(backfill_stale_running_gauge{job="analytics-workers"}) or vector(0)')"
scraper_err="$(scalar '100 * sum(rate(analytics_scraper_requests_total{job="analytics-workers",result="error"}[15m])) / clamp_min(sum(rate(analytics_scraper_requests_total{job="analytics-workers"}[15m])), 0.001)')"
echo ""
echo "stale running jobs: ${stale}"
echo "scraper error rate (15m): ${scraper_err}%"

echo ""
echo "top TT failure reasons (15m rate):"
prom_query 'topk(5, sum by (reason) (rate(analytics_tt_scrape_failures_total{job="analytics-workers",source="twitchtracker"}[15m])))' | python3 -c "
import json,sys
rows=json.load(sys.stdin).get('data',{}).get('result',[])
if not rows:
    print('  (no classified failures yet — deploy analytics-workers with taxonomy metrics)')
for row in sorted(rows, key=lambda r: -float(r['value'][1])):
    print(f\"  {row['metric'].get('reason','?'):24} {float(row['value'][1]):.4f}/s\")
"
