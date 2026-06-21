#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"
PSQL=(docker compose --env-file .env --env-file deploy/env/profile-full.env
  --env-file deploy/env/profile-archive.env --env-file deploy/env/profile-bearhost-prod.env
  -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml
  -f deploy/docker-compose.bearhost-prod.yml -f deploy/docker-compose.bearhost-build.yml
  --profile scraper exec -T postgres psql -U app -d streamclone -Atqc)
"${PSQL[@]}" "SELECT COUNT(*) FROM tracked_streamers;"
"${PSQL[@]}" "WITH roster AS (SELECT login FROM tracked_streamers ORDER BY last_rank ASC NULLS LAST LIMIT 200) SELECT (SELECT COUNT(*) FROM roster), (SELECT COUNT(*) FROM roster r JOIN bronze_index_state b ON b.login = r.login), (SELECT COUNT(*) FROM roster r JOIN bronze_index_state b ON b.login = r.login WHERE b.helix_row_count >= 80);"
"${PSQL[@]}" "WITH roster AS (SELECT login FROM tracked_streamers ORDER BY last_rank ASC NULLS LAST LIMIT 100) SELECT (SELECT COUNT(*) FROM roster), (SELECT COUNT(*) FROM roster r JOIN bronze_index_state b ON b.login = r.login);"
