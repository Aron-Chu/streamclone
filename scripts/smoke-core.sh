#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

run_ui=false
skip_readiness=false
for arg in "$@"; do
  case "$arg" in
    --ui) run_ui=true ;;
    --skip-readiness) skip_readiness=true ;;
  esac
done

fail() {
  echo "smoke-core: $1" >&2
  exit 1
}

wait_all_services() {
  local urls=(
    "http://localhost:8090/|Caddy proxy (frontend)"
    "http://localhost:8081/healthz|metadata"
    "http://localhost:8082/healthz|video"
    "http://localhost:8083/healthz|chat"
    "http://localhost:8084/healthz|emote"
  )
  echo "Checking core services..."
  for i in $(seq 1 60); do
    local all_ok=true
    for entry in "${urls[@]}"; do
      local url="${entry%%|*}"
      if ! curl --connect-timeout 2 --max-time 5 -fsS "$url" >/dev/null 2>&1; then
        all_ok=false
        break
      fi
    done
    if [ "$all_ok" = true ]; then
      echo "  all core services ready (attempt $i)"
      return 0
    fi
    sleep 2
  done
  fail "core services failed — is the stack up? Run 'make bootstrap' or 'make up'."
}

if [ "$skip_readiness" = false ]; then
  wait_all_services
fi

if [ -x "$ROOT/deploy/smoke/test-core-routes-only.sh" ]; then
  echo "Checking core-only route boundary..."
  bash "$ROOT/deploy/smoke/test-core-routes-only.sh"
fi

if [ "$run_ui" = true ]; then
  echo "Running Playwright smoke-core..."
  (cd frontend && npx playwright install chromium >/dev/null && npm run test:smoke)
fi

echo "smoke-core: all checks passed"
