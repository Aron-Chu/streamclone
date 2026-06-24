#!/usr/bin/env bash
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
bash "${DIR}/routes_snapshot.sh"
bash "${DIR}/db_schema_snapshot.sh"
bash "${DIR}/backfill_status.sh"
bash "${DIR}/grafana_snapshot.sh"
echo "context snapshots → runtime/context/"
