#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

run_ui=false
for arg in "$@"; do
  case "$arg" in
    --ui) run_ui=true ;;
  esac
done

fail() {
  echo "smoke-core: $1" >&2
  exit 1
}

wait_url() {
  local url="$1"
  local label="$2"
  echo "Checking $label ($url)..."
  for i in $(seq 1 60); do
    if curl --connect-timeout 2 --max-time 5 -fsS "$url" >/dev/null 2>&1; then
      echo "  ok"
      return 0
    fi
    sleep 2
  done
  fail "$label failed — is the stack up? Run 'make bootstrap' or 'make up'."
}

wait_url "http://localhost:8090/" "Caddy proxy (frontend)"
wait_url "http://localhost:8081/healthz" "metadata"
wait_url "http://localhost:8082/healthz" "video"
wait_url "http://localhost:8083/healthz" "chat"
wait_url "http://localhost:8084/healthz" "emote"
wait_url "http://localhost:8086/healthz" "analytics"

if [ "$run_ui" = true ]; then
  echo "Running Playwright smoke-core..."
  (cd frontend && npm run test:smoke)
fi

echo "smoke-core: all checks passed"
