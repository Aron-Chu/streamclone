#!/usr/bin/env bash
# Core stack health snapshot (compact). Written to runtime/context/backfill_status.txt for agent snapshots.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="${ROOT}/runtime/context"
BASE="${STREAMCLONE_BASE_URL:-http://localhost:8090}"
mkdir -p "$OUT"

{
  echo "# Core stack health — $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo
  echo "## Proxy"
  curl -sf --max-time 5 "${BASE}/" -o /dev/null -w "HTTP %{http_code}\n" 2>/dev/null || echo "(proxy unreachable at ${BASE})"
  echo
  for svc in metadata video chat emote; do
    echo "## ${svc}"
    curl -sf --max-time 5 "${BASE}/v1/${svc}/health" 2>/dev/null | head -c 400 || echo "(health unreachable)"
    echo
    echo
  done
  echo "## Note"
  echo "StreamPulse extension/backfill surfaces live in streamclone-pulse and streampulse-backend — not this core snapshot."
} > "${OUT}/backfill_status.txt"

echo "wrote ${OUT}/backfill_status.txt"
