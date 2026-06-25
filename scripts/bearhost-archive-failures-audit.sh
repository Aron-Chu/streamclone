#!/usr/bin/env bash
# Read-only archive_exports failure audit for Batch B2 (no mutations).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# shellcheck source=scripts/lib/bearhost-compose.sh
source "${ROOT}/scripts/lib/bearhost-compose.sh"

psqlc() {
  bearhost_compose exec -T postgres psql -U app -d streamclone -v ON_ERROR_STOP=1 -c "$1"
}

echo "==> archive_exports by type/status"
psqlc "select artifact_type, export_status, count(*) as exports, max(updated_at) as latest
from archive_exports group by 1,2 order by 1,2;"

echo "==> failed/partial by failure_reason (top 15)"
psqlc "select coalesce(nullif(trim(failure_reason), ''), '(empty)') as failure_reason,
       export_status, count(*) as exports, max(updated_at) as latest
from archive_exports
where export_status in ('failed', 'partial')
group by 1,2 order by exports desc limit 15;"

echo "==> recent failed/partial sample (5 rows, sanitized)"
psqlc "select artifact_type, export_status, left(coalesce(failure_reason, ''), 120) as failure_reason,
       natural_key, updated_at
from archive_exports
where export_status in ('failed', 'partial')
order by updated_at desc limit 5;"

echo "==> failed in last 7 days"
psqlc "select count(*) as failed_7d from archive_exports
where export_status = 'failed' and updated_at > now() - interval '7 days';"

echo "bearhost-archive-failures-audit: done (read-only)"
