#!/usr/bin/env bash
# Assert core Streamclone Caddy serves watch APIs only — no local BFF/analytics routes.
set -euo pipefail

BASE="${SMOKE_BASE:-http://localhost:8090}"

fail() {
  echo "test-core-routes-only: $1" >&2
  exit 1
}

expect_status() {
  local method="$1"
  local path="$2"
  local want="$3"
  local code
  code="$(curl -s -o /dev/null -w '%{http_code}' -X "$method" "${BASE}${path}" --max-time 5 || echo 000)"
  if [ "$code" != "$want" ]; then
    fail "${method} ${path} returned HTTP ${code} (want ${want})"
  fi
  echo "  OK ${method} ${path} -> ${code}"
}

echo "Core route boundary (${BASE})..."

# Core must stay reachable through Caddy.
expect_status GET / 200
expect_status GET /v1/setup/welcome 200

# BFF / analytics paths must not be proxied on the public core stack.
ext="extension"
portal="portal"
for path in \
  "/v1/${ext}/health" \
  /v1/pulse/bookmarks \
  /v1/public/status \
  "/v1/${portal}/analytics/channels/xqc/live" \
  expect_status GET "$path" 404
done

echo "test-core-routes-only: passed"
