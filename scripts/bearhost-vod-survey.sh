#!/usr/bin/env bash
# BearHost VPS: bronze roster progress + coverage snapshot for top 100-200.
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"
export BEARHOST_USE_DOCKER_GO=1

PSQL=(docker compose --env-file .env --env-file deploy/env/profile-full.env
  --env-file deploy/env/profile-archive.env --env-file deploy/env/profile-bearhost-prod.env
  -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml
  -f deploy/docker-compose.bearhost-prod.yml -f deploy/docker-compose.bearhost-build.yml
  --profile scraper exec -T postgres psql -U app -d streamclone)

echo "==> bronze counts"
"${PSQL[@]}" -Atqc "
SELECT 'indexed_total', COUNT(*)::text FROM bronze_index_state
UNION ALL SELECT 'with_helix', COUNT(*)::text FROM bronze_index_state WHERE last_helix_at IS NOT NULL
UNION ALL SELECT 'helix_rows_avg', COALESCE(ROUND(AVG(helix_row_count))::text, '0') FROM bronze_index_state WHERE helix_row_count > 0
UNION ALL SELECT 'at_helix_cap_80', COUNT(*)::text FROM bronze_index_state WHERE helix_row_count >= 80;
"

echo ""
echo "==> tracked_streamers count"
"${PSQL[@]}" -Atqc "SELECT COUNT(*) FROM tracked_streamers;" || echo "n/a"

echo ""
echo "==> top-100 / top-200 bronze coverage"
for n in 100 200; do
  "${PSQL[@]}" -Atqc "
WITH roster AS (
  SELECT login FROM tracked_streamers ORDER BY last_rank ASC NULLS LAST, login ASC LIMIT ${n}
)
SELECT ${n},
  (SELECT COUNT(*) FROM roster r JOIN bronze_index_state b ON b.login = r.login),
  (SELECT COUNT(*) FROM roster r JOIN bronze_index_state b ON b.login = r.login WHERE b.helix_row_count >= 80);
" || echo "top-${n}: n/a"
done

echo ""
echo "==> coverage report (7d stream minute-rollups — not bronze VOD catalog)"
bash "${ROOT}/scripts/bearhost-go-run.sh" ./cmd/backfill coverage report --since=7d 2>/dev/null | python3 -c "
import json,sys
d=json.load(sys.stdin)
r=d.get('roster',{})
ss=d.get('streamSummary',{})
print('roster topN:', r.get('topN'), 'totalTracked:', r.get('totalTracked'))
print('streams since 7d:', ss.get('total'), 'live:', ss.get('live'), 'ended:', ss.get('ended'))
print('minute rollup coverage — live_good:', ss.get('liveGood'), 'partial:', ss.get('partial'), 'tt_required:', ss.get('ttRequired'))
"

echo ""
echo "==> VOD catalog date depth (sample channels)"
for login in xqc sodapoppin cellbit summit1g shroud; do
  if bash "${ROOT}/scripts/bearhost-go-run.sh" ./cmd/backfill bronze vod-range --login="${login}" 2>/dev/null; then
    :
  else
    echo "{\"login\":\"${login}\",\"error\":\"vod-range unavailable or blob missing\"}"
  fi
done
