#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "Stopping Streamclone..."
docker compose --env-file .env \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.local-tunnel.yml \
  --profile scraper \
  down --remove-orphans --timeout 30 2>/dev/null || true

docker compose --env-file .env \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.local-tunnel.yml \
  down --remove-orphans --timeout 30

echo "Stopped."
