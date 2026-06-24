#!/usr/bin/env bash
# Batch R3 Phase 0 — preflight on BearHost before 24h soak.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "${ROOT}"

fail=0
pass() { echo "PASS: $*"; }
fail_msg() { echo "FAIL: $*" >&2; fail=1; }

echo "==> Batch R3 preflight (production soak cap=10)"

if bash scripts/ops-000-vps-check.sh; then
  pass "ops-000 no analytics-workers"
else
  fail_msg "ops-000 blocked — stop corpus before soak"
fi

if bash scripts/ops-001-pulse-metrics-check.sh; then
  pass "ops-001 metrics baseline"
else
  fail_msg "ops-001 metrics check"
fi

cap="$(grep '^PULSE_MAX_ACTIVE_CHANNELS=' deploy/env/profile-bearhost-pulse.env | cut -d= -f2)"
if [[ "${cap}" == "10" ]]; then
  pass "production cap=10"
else
  fail_msg "unexpected cap=${cap} (want 10 before soak)"
fi

if curl -sf http://127.0.0.1:8090/v1/extension/health >/dev/null; then
  pass "extension health :8090"
else
  fail_msg "extension health unreachable"
fi

admin_code="$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8090/v1/admin/pulse/health)"
if [[ "${admin_code}" == "401" ]]; then
  pass "admin pulse 401 without auth"
else
  fail_msg "admin pulse HTTP ${admin_code} (want 401)"
fi

if docker ps --format '{{.Names}}' | grep -qE 'analytics-workers|scraper'; then
  fail_msg "corpus/scraper containers running"
else
  pass "no corpus/scraper containers"
fi

if docker ps --format '{{.Names}}' | grep -q streamclone-pulse-staging; then
  echo "WARN: staging stack running — stopping to free RAM"
  bash scripts/bearhost-pulse-staging-down.sh || true
else
  pass "staging stack not running"
fi

echo ""
if [[ "${fail}" -ne 0 ]]; then
  echo "Batch R3 preflight: FAIL"
  exit 1
fi
echo "Batch R3 preflight: PASS"
exit 0
