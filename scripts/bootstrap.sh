#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=lib/env.sh
source "$ROOT/scripts/lib/env.sh"

cd "$ROOT"
env_preflight_docker

if [ ! -f .env ]; then
  env_synthesize core .env
  echo "Created .env from .env.example + profile-dev + profile-core (secrets generated)."
else
  env_generate_secrets .env
fi

echo "Starting core stack (no scraper/clipper profiles)..."
docker compose --env-file .env \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.local-tunnel.yml \
  up -d --build --remove-orphans

echo ""
echo "Streamclone is starting at http://localhost:8090"
echo "Run 'make smoke' once services are healthy."
