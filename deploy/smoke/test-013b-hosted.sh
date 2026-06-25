#!/usr/bin/env bash
# TEST-013b — hosted admin/Grafana exposure checklist (infra-004 §8).
set -euo pipefail

API_BASE="${TEST013B_API_BASE:-https://api.streampulse.stream}"
GRAFANA_BASE="${TEST013B_GRAFANA_BASE:-https://grafana.streampulse.stream}"
BETA_KEY="${PULSE_BETA_KEY:-}"

fail=0
pass() { echo "PASS: $*"; }
fail_msg() { echo "FAIL: $*" >&2; fail=1; }
warn() { echo "WARN: $*"; }

echo "==> TEST-013b hosted security checklist"
echo "API_BASE=${API_BASE}"
echo "GRAFANA_BASE=${GRAFANA_BASE}"

# 1 — anonymous admin API
code="$(curl -s -o /tmp/admin-anon.json -w '%{http_code}' "${API_BASE}/v1/admin/pulse/health" || echo 000)"
if [[ "${code}" == "200" ]]; then
  if python3 -c "import json; d=json.load(open('/tmp/admin-anon.json')); exit(0 if 'caps' in d else 1)" 2>/dev/null; then
    fail_msg "anonymous admin returned 200 with operator fields"
  else
    warn "admin 200 without caps — investigate"
  fi
elif [[ "${code}" =~ ^(401|403|302|404)$ ]]; then
  pass "anonymous admin blocked (${code})"
else
  fail_msg "anonymous admin unexpected HTTP ${code} (want 401/403/302)"
fi

# 2 — archive admin path (should not be 404 from wrong upstream on pulse host)
code="$(curl -s -o /dev/null -w '%{http_code}' "${API_BASE}/v1/admin/archive/jobs" || echo 000)"
if [[ "${code}" == "404" ]]; then
  pass "archive admin 404 on pulse host (expected)"
elif [[ "${code}" =~ ^(401|403|302)$ ]]; then
  pass "archive admin blocked (${code})"
else
  warn "archive admin HTTP ${code}"
fi

# 3 — anonymous Grafana (unreachable/DNS fail = not public)
gcode="$(curl -s -o /dev/null -w '%{http_code}' "${GRAFANA_BASE}/" 2>/dev/null || true)"
gcode="${gcode:-000}"
if [[ "${gcode}" == "000" || "${gcode}" == "000000" || "${gcode}" =~ ^0+$ ]]; then
  pass "Grafana unreachable (${gcode:-000})"
elif [[ "${gcode}" =~ ^(401|403|302)$ ]]; then
  pass "Grafana not public (${gcode})"
else
  fail_msg "Grafana may be public (HTTP ${gcode})"
fi

# 4 — public health
hcode="$(curl -s -o /tmp/ext-health.json -w '%{http_code}' "${API_BASE}/v1/extension/health" || echo 000)"
if [[ "${hcode}" != "200" ]]; then
  fail_msg "public health HTTP ${hcode} (want 200)"
elif python3 -c "
import json
d=json.load(open('/tmp/ext-health.json'))
assert d.get('ok') is True
for k in ('caps','queues','killSwitches'):
    assert k not in d, k
print('PASS: public health')
" 2>/dev/null; then
  pass "public health shape"
else
  fail_msg "public health shape invalid"
fi

# 5 — public status
code="$(curl -s -o /tmp/status.json -w '%{http_code}' "${API_BASE}/v1/public/status")"
if [[ "${code}" == "200" ]]; then
  pass "public status 200"
else
  fail_msg "public status HTTP ${code}"
fi

# 6 — beta without key
code="$(curl -s -o /dev/null -w '%{http_code}' "${API_BASE}/v1/pulse/watchlist")"
if [[ "${code}" == "401" ]]; then
  pass "beta route 401 without key"
else
  fail_msg "beta route without key HTTP ${code} (want 401)"
fi

# 7 — beta with key (optional)
if [[ -n "${BETA_KEY}" ]]; then
  code="$(curl -s -o /dev/null -w '%{http_code}' -H "X-Streamclone-Beta-Key: ${BETA_KEY}" "${API_BASE}/v1/pulse/watchlist")"
  if [[ "${code}" == "200" ]]; then
    pass "beta route 200 with key"
  else
    fail_msg "beta route with key HTTP ${code} (want 200)"
  fi
else
  warn "PULSE_BETA_KEY unset — skip row 7"
fi

echo ""
if [[ "${fail}" -ne 0 ]]; then
  echo "TEST-013b: FAIL"
  exit 1
fi
echo "TEST-013b: PASS (operator browser check row 10 manual)"
