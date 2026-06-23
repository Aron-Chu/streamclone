#!/usr/bin/env bash
# Silver/gold corpus status on BearHost (Grafana + VPS backfill CLI + failure breakdown).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"

echo "══════════════════════════════════════════════════════════════"
echo " Silver & Gold status — $(date -u +%Y-%m-%dT%H:%MZ)"
echo "══════════════════════════════════════════════════════════════"
echo ""

GRAFANA_USER="${GRAFANA_ADMIN_USER:-admin}"
GRAFANA_PASS="${GRAFANA_ADMIN_PASSWORD:-}"
if [[ -z "${GRAFANA_PASS}" && -f "${ROOT}/deploy/env/.grafana-admin-password" ]]; then
  GRAFANA_PASS="$(tr -d '\r\n' < "${ROOT}/deploy/env/.grafana-admin-password")"
fi

if [[ -n "${GRAFANA_PASS}" ]] && curl -sf -m 5 -u "${GRAFANA_USER}:${GRAFANA_PASS}" \
  "http://127.0.0.1:${GRAFANA_LOCAL_PORT:-3001}/api/health" >/dev/null 2>&1; then
  bash "${ROOT}/scripts/bearhost-archive-status-via-grafana.sh"
else
  echo "(Grafana tunnel down — run: make grafana-up)"
  echo ""
fi

echo ""
echo "──────────────────────────────────────────────────────────────"
echo " VPS backfill summary"
echo "──────────────────────────────────────────────────────────────"
bearhost_ssh_config
bearhost_ssh "cd /opt/streamclone/app && BEARHOST_USE_DOCKER_GO=1 bash scripts/bearhost-go-run.sh ./cmd/backfill status 2>/dev/null" || true

echo ""
echo "──────────────────────────────────────────────────────────────"
echo " Recent failures (silver + gold, last 10)"
echo "──────────────────────────────────────────────────────────────"
bearhost_ssh "cd /opt/streamclone/app && BEARHOST_USE_DOCKER_GO=1 bash scripts/bearhost-go-run.sh ./cmd/backfill jobs list --status=failed --limit=10 2>/dev/null" \
  | python3 -c "
import json,sys
raw=sys.stdin.read()
start=raw.find('[')
if start < 0:
    print('  (no failed jobs list)')
    sys.exit(0)
rows=json.loads(raw[start:raw.rfind(']')+1])
for j in rows:
    err=(j.get('Error') or '')[:90]
    print(f\"  {j.get('Tier','?'):6} {j.get('Login','?'):16} {j.get('StreamID','')}  {err}\")
" 2>/dev/null || true

echo ""
echo "──────────────────────────────────────────────────────────────"
echo " Failure reason breakdown (Postgres, silver + gold failed)"
echo "──────────────────────────────────────────────────────────────"
bearhost_ssh "docker exec streamclone-postgres-1 psql -U app -d streamclone -t -A -c \"
SELECT tier,
  CASE
    WHEN error ILIKE '%backoff%' THEN 'scrape_backoff'
    WHEN error ILIKE '%cloudflare%' OR error ILIKE '%access protected%' THEN 'cloudflare'
    WHEN error ILIKE '%browser%' OR error ILIKE '%window is null%' THEN 'browser_crash'
    WHEN error ILIKE '%missing viewer%' OR error ILIKE '%meta#ecs%' OR error ILIKE '%incomplete%' THEN 'missing_chart'
    WHEN error ILIKE '%session_alias%' OR error ILIKE '%foreign key%' THEN 'session_alias_fk'
    WHEN error ILIKE '%timeout%' OR error ILIKE '%deadline%' THEN 'timeout'
    WHEN error ILIKE '%connection refused%' OR error ILIKE '%unreachable%' THEN 'scraper_down'
    ELSE 'other'
  END AS reason,
  COUNT(*) AS n
FROM analytics_backfill_jobs
WHERE status = 'failed' AND tier IN ('silver','gold')
GROUP BY 1, 2
ORDER BY 1, n DESC;
\"" 2>/dev/null || echo "  (postgres query failed)"

echo ""
echo "──────────────────────────────────────────────────────────────"
echo " Currently running"
echo "──────────────────────────────────────────────────────────────"
bearhost_ssh "cd /opt/streamclone/app && BEARHOST_USE_DOCKER_GO=1 bash scripts/bearhost-go-run.sh ./cmd/backfill jobs list --status=running --limit=5 2>/dev/null" \
  | python3 -c "
import json,sys
raw=sys.stdin.read()
start=raw.find('[')
if start < 0:
    print('  (none)')
    sys.exit(0)
rows=json.loads(raw[start:raw.rfind(']')+1])
if not rows:
    print('  (none)')
for j in rows:
    print(f\"  {j.get('Tier','?'):6} {j.get('Login','?'):16} {j.get('StreamID','')}  attempt={j.get('Attempt',0)}\")
" 2>/dev/null || true

echo ""
echo "Dashboard: http://localhost:3001/d/streamclone-archive/streamclone-archive"
