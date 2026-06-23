#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"

bearhost_ssh 'docker exec streamclone-postgres-1 psql -U app -d streamclone -c "
SELECT tier, status, COUNT(*) AS n
FROM backfill_jobs
WHERE tier IN ('\''silver'\'','\''gold'\'')
GROUP BY 1, 2
ORDER BY 1, 2;
"'

bearhost_ssh 'docker exec streamclone-postgres-1 psql -U app -d streamclone -c "
SELECT tier,
  CASE
    WHEN error ILIKE '\''%backoff%'\'' THEN '\''scrape_backoff'\''
    WHEN error ILIKE '\''%cloudflare%'\'' OR error ILIKE '\''%access protected%'\'' THEN '\''cloudflare'\''
    WHEN error ILIKE '\''%browser%'\'' OR error ILIKE '\''%window is null%'\'' THEN '\''browser_crash'\''
    WHEN error ILIKE '\''%missing viewer%'\'' OR error ILIKE '\''%incomplete%'\'' THEN '\''missing_chart'\''
    WHEN error ILIKE '\''%session_alias%'\'' OR error ILIKE '\''%foreign key%'\'' THEN '\''session_alias_fk'\''
    WHEN error ILIKE '\''%timeout%'\'' OR error ILIKE '\''%deadline%'\'' THEN '\''timeout'\''
    ELSE '\''other'\''
  END AS reason,
  COUNT(*) AS n
FROM backfill_jobs
WHERE status = '\''failed'\'' AND tier IN ('\''silver'\'','\''gold'\'')
GROUP BY 1, 2
ORDER BY 1, n DESC
LIMIT 20;
"'
