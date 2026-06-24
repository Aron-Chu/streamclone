#!/usr/bin/env bash
# Local smoke: pulse admin auth via archive token (no Cloudflare Access).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
BASE="${PULSE_ADMIN_SMOKE_BASE:-http://127.0.0.1:8090}"
TOKEN="${ADMIN_ARCHIVE_TOKEN:-local-admin-smoke}"

export ADMIN_ARCHIVE_ENABLED=true
export ADMIN_ARCHIVE_REQUIRE_TOKEN=true
export ADMIN_ARCHIVE_TOKEN="${TOKEN}"

echo "==> pulse admin auth local smoke: ${BASE}"

# Public health must not expose operator fields
python3 - "${BASE}" <<'PY'
import json, sys, urllib.request
base = sys.argv[1]
with urllib.request.urlopen(base + "/v1/extension/health", timeout=10) as r:
    data = json.load(r)
for key in ("caps", "queues", "killSwitches"):
    if key in data:
        raise SystemExit(f"FAIL: public health exposes {key}")
print("OK: public health shape")
PY

code="$(curl -s -o /dev/null -w '%{http_code}' "${BASE}/v1/admin/pulse/health")"
if [[ "${code}" != "401" ]]; then
  echo "FAIL: admin without token expected 401, got ${code}" >&2
  exit 1
fi

code="$(curl -s -o /tmp/admin-health.json -w '%{http_code}' \
  -H "X-Admin-Archive-Token: ${TOKEN}" \
  "${BASE}/v1/admin/pulse/health")"
if [[ "${code}" != "200" ]]; then
  echo "FAIL: admin with token expected 200, got ${code}" >&2
  cat /tmp/admin-health.json >&2 || true
  exit 1
fi

python3 - /tmp/admin-health.json <<'PY'
import json, sys
with open(sys.argv[1]) as f:
    data = json.load(f)
if "caps" not in data or "queues" not in data:
    raise SystemExit("FAIL: admin health missing caps/queues")
print("OK: admin health includes operator fields")
PY

echo "OK: pulse admin auth local smoke passed"
