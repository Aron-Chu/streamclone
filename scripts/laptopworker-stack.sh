#!/usr/bin/env bash
# Remote start/stop for laptopworker dev stack (docker compose — not machine power).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck source=laptopworker-env.sh
source "$ROOT/scripts/laptopworker-env.sh"

if ! laptopworker_required_files "$ROOT"; then
  echo "laptopworker overlay missing — see docs/laptopworker-dev.md" >&2
  exit 1
fi

ENV_FILE="${ENV_FILE:-.env}"
COMPOSE=(docker compose --env-file "$ENV_FILE"
  -f deploy/docker-compose.yml
  -f deploy/docker-compose.local-tunnel.yml
  -f deploy/docker-compose.laptopworker-dev.yml)

if [ -f deploy/env/.env.pulse-local ]; then
  COMPOSE+=(--env-file deploy/env/.env.pulse-local)
fi
if [ -f .env.local ]; then
  COMPOSE+=(--env-file .env.local)
fi

_laptopworker_up() {
  "${COMPOSE[@]}" up -d --remove-orphans
  laptopworker_stop_storygraph
}

usage() {
  cat <<'EOF'
Usage: scripts/laptopworker-stack.sh <command>

  start    Start core dev stack (detached)
  stop     Stop containers (machine stays awake)
  restart  stop then start
  status   docker compose ps
  logs     Follow compose logs (Ctrl+C)
  smoke    Health via :8090 extension + frontend
  update   git pull + resynth .env + compose up (after push to master)
  install-service  systemd user unit + boot linger (run once on laptop)
EOF
}

cmd="${1:-}"
case "$cmd" in
  start)
    _laptopworker_up
    "${COMPOSE[@]}" ps
    ;;
  stop)
    "${COMPOSE[@]}" stop
    laptopworker_stop_storygraph
    ;;
  restart)
    "${COMPOSE[@]}" stop
    _laptopworker_up
    "${COMPOSE[@]}" ps
    ;;
  status)
    "${COMPOSE[@]}" ps
    ;;
  logs)
    "${COMPOSE[@]}" logs -f --tail=100
    ;;
  smoke)
    curl -fsS "http://127.0.0.1:8090/v1/extension/health" | head -c 240
    echo
    code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8090/)"
    echo "OK frontend / → HTTP $code"
    ;;
  update)
    bash "$ROOT/scripts/laptopworker-update.sh"
    ;;
  install-service)
    bash "$ROOT/scripts/laptopworker-install-service.sh"
    ;;
  -h|--help|"")
    usage
    [ -n "$cmd" ] || exit 1
    ;;
  *)
    echo "Unknown command: $cmd" >&2
    usage >&2
    exit 1
    ;;
esac
