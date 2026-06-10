#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker is required. Install Docker Desktop and ensure 'docker' is on PATH."
  exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
  echo "docker compose is required. Update Docker Desktop."
  exit 1
fi

if [ ! -f .env ]; then
  cp .env.dev .env
  if command -v openssl >/dev/null 2>&1; then
    token="$(openssl rand -hex 24)"
  else
    token="$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  fi
  if [ "$(uname)" = "Darwin" ]; then
    sed -i '' "s/^CURATOR_API_TOKEN=.*/CURATOR_API_TOKEN=${token}/" .env
  else
    sed -i "s/^CURATOR_API_TOKEN=.*/CURATOR_API_TOKEN=${token}/" .env
  fi
  echo "Created .env from .env.dev (random CURATOR_API_TOKEN)."
fi

echo "Starting core stack (no scraper/clipper profiles)..."
docker compose --env-file .env \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.local-tunnel.yml \
  up -d --build --remove-orphans

echo ""
echo "Streamclone is starting at http://localhost:8090"
echo "Run 'make smoke' once services are healthy."
