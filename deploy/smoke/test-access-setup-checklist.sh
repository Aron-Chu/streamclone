#!/usr/bin/env bash
# INFRA-004 T1 — Cloudflare Access setup verification (operator checklist).
# Does not configure Access (dashboard-only); verifies current exposure matches runbook expectations.
set -euo pipefail

API_BASE="${ACCESS_CHECK_API_BASE:-https://api.streampulse.stream}"
PAGES_BASE="${ACCESS_CHECK_PAGES_BASE:-https://streampulse.stream}"
GRAFANA_BASE="${ACCESS_CHECK_GRAFANA_BASE:-https://grafana.streampulse.stream}"

fail=0
pass() { echo "PASS: $*"; }
warn() { echo "WARN: $*"; }
fail_msg() { echo "FAIL: $*" >&2; fail=1; }

echo "==> INFRA-004 Access setup checklist"
echo "API_BASE=${API_BASE}"
echo "PAGES_BASE=${PAGES_BASE}"
echo "GRAFANA_BASE=${GRAFANA_BASE}"
echo ""
echo "Operator steps (Cloudflare Zero Trust dashboard):"
echo "  1. Identity provider + group streampulse-operators"
echo "  2. Access app: api.streampulse.stream path /v1/admin/*"
echo "  3. Access app: grafana.streampulse.stream (after tunnel hostname)"
echo "  4. Access app: streampulse.stream path /admin/* (after Pages deploy)"
echo "  5. Copy Application AUD → PULSE_CF_ACCESS_AUD on BearHost (post-soak redeploy)"
echo ""

# Anonymous admin API — should not return 200 with operator fields
code="$(curl -s -o /tmp/access-admin.json -w '%{http_code}' "${API_BASE}/v1/admin/pulse/health" || echo 000)"
if [[ "${code}" == "200" ]]; then
  fail_msg "anonymous admin returned 200 — configure Access on /v1/admin/*"
elif [[ "${code}" =~ ^(401|403|302|404)$ ]]; then
  pass "anonymous admin blocked (${code}) — edge or app auth working"
else
  warn "anonymous admin HTTP ${code}"
fi

# Pages /admin — expect Access block or SPA shell (not public operator data)
code="$(curl -s -o /dev/null -w '%{http_code}' "${PAGES_BASE}/admin/" || echo 000)"
if [[ "${code}" =~ ^(200|401|403|302)$ ]]; then
  pass "Pages /admin reachable (${code}) — verify Access policy in dashboard"
else
  warn "Pages /admin HTTP ${code} — deploy streampulse-web /admin if missing"
fi

# Grafana tunnel
code="$(curl -s -o /dev/null -w '%{http_code}' "${GRAFANA_BASE}/" || echo 000)"
if [[ "${code}" =~ ^(401|403|302)$ ]]; then
  pass "Grafana blocked at edge (${code})"
elif [[ "${code}" == "000" ]]; then
  warn "Grafana unreachable — add tunnel hostname after Access app exists"
else
  warn "Grafana HTTP ${code} — confirm Access policy before exposing"
fi

echo ""
if [[ "${fail}" -eq 0 ]]; then
  echo "ACCESS-CHECK: PASS (dashboard policies still require manual confirmation)"
  exit 0
fi
echo "ACCESS-CHECK: FAIL — fix exposure before operator sign-off"
exit 1
