#!/usr/bin/env bash
# OPS-000 — Local archive isolation preflight before deploy.
# Hosted VPS archive preflight moved to private streampulse-ops.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

if [[ "${OPS_000_HOSTED:-}" == "1" ]]; then
  exec bash "${ROOT}/scripts/ops-stub.sh"
fi

echo "==> OPS-000: local archive isolation preflight"
echo "NOTE: hosted production archive checks live in streampulse-ops (not this repo)."

fail=0
warn() { echo "WARN: $*" >&2; }
pass() { echo "PASS: $*"; }
block() { echo "BLOCK: $*" >&2; fail=1; }

if command -v docker >/dev/null 2>&1; then
  if docker ps --format '{{.Names}}' 2>/dev/null | grep -q 'analytics-workers'; then
    corpus_env="$(docker exec streamclone-analytics-workers env 2>/dev/null | grep -E '^(CORPUS|BRONZE|SILVER|BACKFILL)_' || true)"
    if echo "${corpus_env}" | grep -qE 'CORPUS_WORKERS_ENABLED=(1|true)'; then
      block "analytics-workers container has corpus enabled — wait for archive jobs before deploy"
      echo "${corpus_env}" | head -20
    else
      pass "analytics-workers corpus flags off (or container not corpus-active)"
    fi
  else
    pass "analytics-workers not running locally (skip container corpus check)"
  fi
else
  warn "docker not available — skip live container checks"
fi

if [[ "${fail}" -ne 0 ]]; then
  echo "OPS-000: FAILED — resolve blocks before deploy" >&2
  exit 1
fi

echo "OPS-000: preflight complete"
