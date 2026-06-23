#!/usr/bin/env bash
# Post-deploy smoke for TT scrape backoff + taxonomy metrics on BearHost.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"

bearhost_ssh_config
bearhost_ssh 'set -e
echo "==> TT env (analytics-workers)"
docker inspect streamclone-analytics-workers --format "{{range .Config.Env}}{{println .}}{{end}}" | grep ANALYTICS_TT || true
echo ""
echo "==> scraper health"
docker exec streamclone-scraper curl -sf http://127.0.0.1:8000/health
echo ""
echo "==> Prometheus TT taxonomy (5m rate)"
curl -sf -G http://127.0.0.1:9090/api/v1/query \
  --data-urlencode "query=topk(5, sum by (reason) (rate(analytics_tt_scrape_failures_total{job=\"analytics-workers\",source=\"twitchtracker\"}[5m])))" \
  | python3 -c "import json,sys; r=json.load(sys.stdin).get(\"data\",{}).get(\"result\",[]); print(\"  (none yet)\") if not r else [print(f\"  {x[\"metric\"].get(\"reason\",\"?\"):24} {float(x[\"value\"][1]):.4f}/s\") for x in sorted(r,key=lambda v:-float(v[\"value\"][1]))]" 2>/dev/null || echo "  (prometheus query failed)"
echo ""
echo "==> scraper error rate (15m)"
curl -sf -G http://127.0.0.1:9090/api/v1/query \
  --data-urlencode "query=100 * sum(rate(analytics_scraper_requests_total{job=\"analytics-workers\",result=\"error\"}[15m])) / clamp_min(sum(rate(analytics_scraper_requests_total{job=\"analytics-workers\"}[15m])), 0.001)" \
  | python3 -c "import json,sys; r=json.load(sys.stdin).get(\"data\",{}).get(\"result\",[]); print(r[0][\"value\"][1]+\"%\" if r else \"n/a\")" 2>/dev/null || echo "n/a"
echo ""
echo "==> recent worker TT activity"
docker logs --tail=40 streamclone-analytics-workers 2>&1 | grep -iE "TwitchTracker|backoff|viewer_status|scrape failed|live rollups" | tail -15 || true
'
