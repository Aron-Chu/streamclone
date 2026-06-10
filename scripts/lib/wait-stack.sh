#!/usr/bin/env bash
set -euo pipefail

URL="${1:-http://localhost:8090/}"
TIMEOUT_SEC="${2:-300}"
INTERVAL_SEC="${3:-3}"

echo "Waiting for Streamclone at $URL (up to ${TIMEOUT_SEC}s)..."
deadline=$((SECONDS + TIMEOUT_SEC))
attempt=0

while [ "$SECONDS" -lt "$deadline" ]; do
  attempt=$((attempt + 1))
  if curl -fsS -o /dev/null "$URL" 2>/dev/null; then
    echo "  Streamclone is ready (attempt $attempt)"
    exit 0
  fi
  if [ $((attempt % 5)) -eq 0 ]; then
    echo "  still starting... (attempt $attempt)"
  fi
  sleep "$INTERVAL_SEC"
done

echo ""
echo "Streamclone did not become ready in time."
echo "Try: docker compose --env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml ps"
echo "See: docs/install-desktop.md"
exit 1
