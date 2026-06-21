#!/usr/bin/env bash
# Audit Grafana dashboards vs datasources/metrics on the BearHost tunnel port.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PORT="${BEARHOST_GRAFANA_LOCAL_PORT:-3001}"
if [[ -z "${GRAFANA_ADMIN_USER:-}" || -z "${GRAFANA_ADMIN_PASSWORD:-}" ]]; then
  echo "Set GRAFANA_ADMIN_USER and GRAFANA_ADMIN_PASSWORD for Grafana API access." >&2
  exit 1
fi
GRAFANA_AUTH=(-u "${GRAFANA_ADMIN_USER}:${GRAFANA_ADMIN_PASSWORD}")

echo "=== datasources on :${PORT} ==="
DS="$(curl -sf "${GRAFANA_AUTH[@]}" "http://127.0.0.1:${PORT}/api/datasources" || echo '[]')"
echo "$DS" | python3 -m json.tool

for uid in streamclone-archive streamclone-ops streamclone-pulse-wire streamclone-emote-pulse; do
  echo ""
  echo "=== dashboard $uid ==="
  DASH="$(curl -sf "${GRAFANA_AUTH[@]}" "http://127.0.0.1:${PORT}/api/dashboards/uid/${uid}" || echo '{}')"
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
