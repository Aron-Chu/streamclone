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
    "http://localhost:8086/healthz|analytics"
    "http://localhost:8087/healthz|storygraph"
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

if [ -f .env ] && grep -q '^PULSE_WIRE_ENABLED=true' .env 2>/dev/null; then
  feed_code="$(curl --connect-timeout 2 --max-time 5 -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:8090/v1/pulse-wire/feed" 2>/dev/null || echo 000)"
  if [ "$feed_code" = "200" ]; then
    echo "  Pulse Wire feed proxy ok"
  else
    echo "  Pulse Wire feed not reachable through proxy (optional, http $feed_code)" >&2
  fi
fi

if [ "$run_ui" = true ]; then
  echo "Running Playwright smoke-core..."
  (cd frontend && npx playwright install chromium >/dev/null && npm run test:smoke)
fi

echo "smoke-core: all checks passed"
