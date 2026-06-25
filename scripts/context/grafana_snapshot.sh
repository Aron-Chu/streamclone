#!/usr/bin/env bash
# Grafana dashboard list when pulse Helm/profile is up (optional).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="${ROOT}/runtime/context"
GRAFANA="${GRAFANA_URL:-http://localhost:3000}"
mkdir -p "$OUT"

{
  echo "# Grafana snapshot — $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "# URL: ${GRAFANA}"
  echo
  if curl -sf --max-time 5 "${GRAFANA}/api/health" >/dev/null 2>&1; then
    curl -sf --max-time 8 "${GRAFANA}/api/search?type=dash-db&limit=20" 2>/dev/null \
      | python3 -c "import json,sys; d=json.load(sys.stdin); print('\n'.join(f\"- {x.get('title')} ({x.get('uid')})\" for x in d[:20]))" \
      2>/dev/null || echo "(dashboard list parse failed — auth may be required)"
  else
    echo "grafana: not reachable (optional — make pulse-on or helm-grafana)"
  fi
} > "${OUT}/grafana.txt"

echo "wrote ${OUT}/grafana.txt"
