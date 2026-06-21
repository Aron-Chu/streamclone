#!/usr/bin/env bash
set -euo pipefail
cd /opt/streamclone/app

COMPOSE=(docker compose
  --env-file .env
  --env-file deploy/env/profile-full.env
  --env-file deploy/env/profile-archive.env
  --env-file deploy/env/profile-bearhost-prod.env
  -f deploy/docker-compose.yml
  -f deploy/docker-compose.release.yml
  -f deploy/docker-compose.prod.yml
  -f deploy/docker-compose.bearhost-prod.yml
  --profile scraper
)

echo "==> Phase 1: postgres + redis"
"${COMPOSE[@]}" up -d postgres redis
for i in $(seq 1 30); do
  if "${COMPOSE[@]}" exec -T postgres pg_isready -U app -d streamclone >/dev/null 2>&1; then
    break
  fi
  sleep 2
done
"${COMPOSE[@]}" ps postgres redis

echo "==> Phase 2: migrate"
"${COMPOSE[@]}" run --rm migrate

echo "==> Phase 3: scraper"
"${COMPOSE[@]}" pull scraper
"${COMPOSE[@]}" up -d scraper
for i in $(seq 1 60); do
  if "${COMPOSE[@]}" exec -T scraper curl -sf http://127.0.0.1:8000/health >/dev/null 2>&1; then
    echo "scraper healthy"
    break
  fi
  sleep 5
done

echo "==> Phase 4: metadata + analytics + workers"
"${COMPOSE[@]}" pull metadata analytics analytics-workers
"${COMPOSE[@]}" up -d metadata analytics analytics-workers

echo "==> Phase 5: chat video emote frontend + minio mediamtx"
"${COMPOSE[@]}" pull chat video emote frontend
"${COMPOSE[@]}" up -d chat video emote frontend minio mediamtx

echo "==> Phase 6: caddy"
"${COMPOSE[@]}" up -d caddy
sleep 8
"${COMPOSE[@]}" ps
