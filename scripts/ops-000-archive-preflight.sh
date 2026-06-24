#!/usr/bin/env bash
# OPS-000 — Archive isolation preflight before BearHost Pulse deploy.
# Ensures Pulse deploy does not restart corpus/archive workers mid-job.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

# shellcheck source=scripts/lib/bearhost-compose.sh
source "${ROOT}/scripts/lib/bearhost-compose.sh"

echo "==> OPS-000: archive isolation preflight"

fail=0
warn() { echo "WARN: $*" >&2; }
pass() { echo "PASS: $*"; }
block() { echo "BLOCK: $*" >&2; fail=1; }

if [[ -f deploy/env/profile-bearhost-pulse.env ]]; then
  if grep -qE '^CORPUS_WORKERS_ENABLED=(1|true)' deploy/env/profile-bearhost-pulse.env 2>/dev/null; then
    block "profile-bearhost-pulse.env has CORPUS_WORKERS_ENABLED enabled — Pulse profile must keep corpus off"
  else
    pass "profile-bearhost-pulse.env keeps corpus workers off"
  fi
else
  warn "deploy/env/profile-bearhost-pulse.env not found locally"
fi

if command -v docker >/dev/null 2>&1; then
  if docker ps --format '{{.Names}}' 2>/dev/null | grep -q 'analytics-workers'; then
    corpus_env="$(docker exec streamclone-analytics-workers env 2>/dev/null | grep -E '^(CORPUS|BRONZE|SILVER|BACKFILL)_' || true)"
    if echo "${corpus_env}" | grep -qE 'CORPUS_WORKERS_ENABLED=(1|true)'; then
      block "analytics-workers container has corpus enabled — check archive job status before Pulse deploy"
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

echo ""
echo "Manual VPS checks before bearhost-pulse-api.sh:"
echo "  1. Confirm bronze/silver/gold archive jobs not mid-flight (Grafana archive dashboard)"
echo "  2. bearhost-pulse-api.sh stops analytics-workers — wait if archive jobs running"
echo "  3. Re-run deploy/smoke/bearhost-pulse-api.sh after deploy"

if [[ "${fail}" -ne 0 ]]; then
  echo "OPS-000: FAILED — resolve blocks before Pulse deploy" >&2
  exit 1
fi

echo "OPS-000: preflight complete (resolve manual VPS items on BearHost)"
