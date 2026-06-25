#!/usr/bin/env bash
# Batch Q post-deploy canary — no beta keys printed.
set -euo pipefail

BASE="${PULSE_SMOKE_BASE_URL:-https://api.streampulse.stream}"
BASE="${BASE%/}"

echo "==> public health (no auth)"
curl -sf "${BASE}/v1/extension/health" | python3 -m json.tool 2>/dev/null || curl -sf "${BASE}/v1/extension/health"
echo ""

echo "==> public status (no auth)"
curl -sf "${BASE}/v1/public/status" | python3 -m json.tool 2>/dev/null || curl -sf "${BASE}/v1/public/status"
echo ""

echo "==> gated bookmarks without key (expect 401)"
code="$(curl -s -o /dev/null -w '%{http_code}' "${BASE}/v1/pulse/bookmarks")"
echo "HTTP ${code}"
if [[ "${code}" != "401" ]]; then
  echo "FAIL: expected 401 without beta key" >&2
  exit 1
fi

if [[ -n "${PULSE_BETA_KEY:-}" ]]; then
  echo "==> gated bookmarks with key (expect 200)"
  code="$(curl -s -o /dev/null -w '%{http_code}' -H "X-Streamclone-Beta-Key: ${PULSE_BETA_KEY}" "${BASE}/v1/pulse/bookmarks")"
  echo "HTTP ${code}"
  if [[ "${code}" != "200" ]]; then
    echo "FAIL: expected 200 with valid beta key" >&2
    exit 1
  fi
else
  echo "SKIP: PULSE_BETA_KEY not set — bookmarks with-key check skipped"
fi

echo "==> admin health anonymous (expect non-200 or Access block; not operator payload)"
admin_code="$(curl -s -o /tmp/admin-health.json -w '%{http_code}' "${BASE}/v1/admin/pulse/health" || true)"
echo "HTTP ${admin_code}"
if [[ "${admin_code}" == "200" ]] && grep -q '"caps"' /tmp/admin-health.json 2>/dev/null; then
  echo "FAIL: anonymous admin health returned operator caps" >&2
  exit 1
fi
rm -f /tmp/admin-health.json

echo "==> grafana hostname (expect non-200 HTML login without Access)"
grafana_code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 https://grafana.streampulse.stream/ 2>/dev/null || echo "000")"
echo "grafana HTTP ${grafana_code} (000=unreachable is OK)"

echo "OK: post-deploy canary passed"
