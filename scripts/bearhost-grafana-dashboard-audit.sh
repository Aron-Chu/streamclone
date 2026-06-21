#!/usr/bin/env bash
# Audit Grafana dashboards vs datasources/metrics on the BearHost tunnel port.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PORT="${BEARHOST_GRAFANA_LOCAL_PORT:-3001}"
AUTH='Authorization: Basic YWRtaW46c3RyZWFtcHVsc2U='

echo "=== datasources on :${PORT} ==="
DS="$(curl -sf -H "$AUTH" "http://127.0.0.1:${PORT}/api/datasources" || echo '[]')"
echo "$DS" | python3 -m json.tool

for uid in streamclone-archive streamclone-ops streamclone-pulse-wire streamclone-emote-pulse; do
  echo ""
  echo "=== dashboard $uid ==="
  DASH="$(curl -sf -H "$AUTH" "http://127.0.0.1:${PORT}/api/dashboards/uid/${uid}" || echo '{}')"
  echo "$DASH" | python3 -c "
import json,sys
d=json.load(sys.stdin).get('dashboard',{})
panels=d.get('panels',[])
ds=set()
for p in panels:
    x=p.get('datasource')
    if isinstance(x,dict): ds.add((x.get('type'), x.get('uid'), x.get('name')))
    elif x: ds.add(('name', x, x))
print('title:', d.get('title'))
print('panel_count:', len(panels))
print('datasources:', sorted(ds))
" 2>/dev/null || echo "dashboard missing"
done

echo ""
echo "=== prometheus metric names (sample) ==="
METRICS="$(curl -sfG "http://127.0.0.1:${PORT}/api/datasources/proxy/uid/prometheus/api/v1/label/__name__/values" -H "$AUTH" 2>/dev/null || echo '{}')"
echo "$METRICS" | python3 -c "
import json,sys
d=json.load(sys.stdin)
names=sorted(d.get('data',[]))
keys=['bronze','archive','analytics','scraper','emote','storygraph','pulse']
for k in keys:
    m=[n for n in names if k in n.lower()]
    print(k+':', len(m), 'metrics', m[:5])
print('total:', len(names))
" 2>/dev/null || echo metrics_err

echo ""
echo "=== sample ops queries ==="
for q in 'analytics_sync_active' 'analytics_scraper_requests_total' 'analytics_rollup_rows_written_total' 'emote_jobs_pending'; do
  n=$(curl -sfG -H "$AUTH" "http://127.0.0.1:${PORT}/api/datasources/proxy/uid/prometheus/api/v1/query" --data-urlencode "query=$q" | python3 -c "import json,sys; print(len(json.load(sys.stdin).get('data',{}).get('result',[])))" 2>/dev/null || echo err)
  echo "$q -> $n series"
done

echo ""
echo "=== archive panel queries ==="
for q in 'bronze_channels_indexed_total' 'bronze_channels_target' 'sum(streamclone_archive_jobs_running)' 'sum(streamclone_archive_jobs_queued)' 'streamclone_archive_coverage_ratio' 'backfill_jobs_gauge'; do
  n=$(curl -sfG -H "$AUTH" "http://127.0.0.1:${PORT}/api/datasources/proxy/uid/prometheus/api/v1/query" --data-urlencode "query=$q" | python3 -c "import json,sys; r=json.load(sys.stdin).get('data',{}).get('result',[]); print(len(r), [x.get('value',[None,None])[1] for x in r[:3]])" 2>/dev/null || echo err)
  echo "$q -> $n"
done

echo ""
echo "=== ops timeseries metrics exist? ==="
for q in 'timeseries_write_attempts_total' 'analytics_vod_gql_backoff_seconds_total' 'analytics_rollup_write_duration_seconds_count'; do
  n=$(curl -sfG -H "$AUTH" "http://127.0.0.1:${PORT}/api/datasources/proxy/uid/prometheus/api/v1/query" --data-urlencode "query=$q" | python3 -c "import json,sys; print(len(json.load(sys.stdin).get('data',{}).get('result',[])))" 2>/dev/null || echo err)
  echo "$q -> $n series"
done

echo ""
echo "=== pulse-wire queries ==="
for q in 'up{job="storygraph"}' 'storygraph_items_ingested_total'; do
  n=$(curl -sfG -H "$AUTH" "http://127.0.0.1:${PORT}/api/datasources/proxy/uid/prometheus/api/v1/query" --data-urlencode "query=$q" | python3 -c "import json,sys; print(len(json.load(sys.stdin).get('data',{}).get('result',[])))" 2>/dev/null || echo err)
  echo "$q -> $n series"
done
