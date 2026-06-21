#!/usr/bin/env bash
set -euo pipefail
cd /opt/streamclone/app

ROOT="$(pwd)"
# shellcheck source=scripts/bearhost-corpus-preflight.sh
source "${ROOT}/scripts/bearhost-corpus-preflight.sh"

bearhost_build_local() {
  if [[ -n "${BEARHOST_BUILD_LOCAL:-}" ]]; then
    [[ "${BEARHOST_BUILD_LOCAL}" == "1" ]]
    return
  fi
  if [[ -f deploy/env/profile-bearhost-prod.env ]] \
    && grep -qE '^BEARHOST_BUILD_LOCAL=1' deploy/env/profile-bearhost-prod.env; then
    return 0
  fi
  return 1
}

COMPOSE=(docker compose
  --env-file .env
  --env-file deploy/env/profile-full.env
  --env-file deploy/env/profile-archive.env
  --env-file deploy/env/profile-bearhost-prod.env
  -f deploy/docker-compose.yml
  -f deploy/docker-compose.prod.yml
  -f deploy/docker-compose.bearhost-prod.yml
  --profile scraper
)

if bearhost_build_local; then
  echo "==> Build-local mode (BEARHOST_BUILD_LOCAL=1) — no GHCR pull"
  COMPOSE+=(-f deploy/docker-compose.bearhost-build.yml)
else
  echo "==> GHCR release mode — docker compose pull"
  COMPOSE+=(-f deploy/docker-compose.release.yml)
fi

bearhost_up_build() {
  local services=("$@")
  if bearhost_build_local; then
    "${COMPOSE[@]}" build "${services[@]}"
    "${COMPOSE[@]}" up -d "${services[@]}"
  else
    "${COMPOSE[@]}" pull "${services[@]}"
    "${COMPOSE[@]}" up -d "${services[@]}"
  fi
}

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
bearhost_up_build scraper
for i in $(seq 1 60); do
  if "${COMPOSE[@]}" exec -T scraper curl -sf http://127.0.0.1:8000/health >/dev/null 2>&1; then
    echo "scraper healthy"
    break
  fi
  sleep 5
done

echo "==> Corpus preflight (analytics-workers)"
if bearhost_corpus_preflight; then
  if [[ "${CORPUS_WORKERS_ENABLED:-false}" == "1" || "${CORPUS_WORKERS_ENABLED:-}" == "true" ]]; then
    export CORPUS_WORKERS_ENABLED=1
    echo "Corpus preflight passed — analytics-workers corpus plane enabled"
  else
    export CORPUS_WORKERS_ENABLED=0
    echo "Corpus preflight passed — CORPUS_WORKERS_ENABLED=false (API/sync only)"
  fi
else
  export CORPUS_WORKERS_ENABLED=0
  echo "Corpus preflight failed — analytics-workers corpus-off (fail closed)"
fi

echo "==> Phase 4: metadata + analytics + workers"
bearhost_up_build metadata analytics analytics-workers

echo "==> Phase 5: chat video emote frontend + minio mediamtx"
bearhost_up_build chat video emote frontend
"${COMPOSE[@]}" up -d minio mediamtx

echo "==> Phase 6: caddy"
"${COMPOSE[@]}" up -d caddy
sleep 8
"${COMPOSE[@]}" ps
