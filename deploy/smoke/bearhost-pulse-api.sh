#!/usr/bin/env bash
# BearHost Pulse public health smoke gate.
#
# Asserts /v1/extension/health is safe for public consumption and catches stale
# deploys (version=dev, helixEnabled=null, vod-hint missing, operator fields leaked).
#
# Usage (local Caddy on VPS):
#   PULSE_SMOKE_BASE_URL=http://127.0.0.1:8090 ./deploy/smoke/bearhost-pulse-api.sh
#
# Usage (hosted API after Cloudflare tunnel):
#   PULSE_SMOKE_BASE_URL=https://api.streampulse.stream \
#     PULSE_EXPECT_HOSTED_MODE=true \
#     ./deploy/smoke/bearhost-pulse-api.sh
#
# Exit nonzero on any assertion failure. Run before and after BearHost deploys.
set -euo pipefail

BASE="${PULSE_SMOKE_BASE_URL:-http://127.0.0.1:8090}"
BASE="${BASE%/}"
URL="${BASE}/v1/extension/health"
EXPECT_HOSTED="${PULSE_EXPECT_HOSTED_MODE:-false}"

echo "==> pulse public health smoke: ${URL}"

tmp="$(mktemp)"
trap 'rm -f "${tmp}"' EXIT

http_code="$(curl -sS -o "${tmp}" -w '%{http_code}' "${URL}" || true)"
if [[ "${http_code}" != "200" ]]; then
  echo "FAIL: HTTP ${http_code} from ${URL}" >&2
  cat "${tmp}" >&2 || true
  exit 1
fi

python3 - "${tmp}" "${EXPECT_HOSTED}" <<'PY'
import json, sys

path, expect_hosted = sys.argv[1], sys.argv[2].lower() in ("1", "true", "yes")
forbidden = {"caps", "rates", "killSwitches", "queues", "config", "secrets"}

try:
    with open(path, encoding="utf-8") as f:
        data = json.load(f)
except json.JSONDecodeError as e:
    print(f"FAIL: invalid JSON: {e}", file=sys.stderr)
    sys.exit(1)

def fail(msg: str) -> None:
    print(f"FAIL: {msg}", file=sys.stderr)
    sys.exit(1)

if data.get("ok") is not True:
    fail(f"ok !== true (got {data.get('ok')!r})")

version = data.get("version")
if not version or version == "dev":
    fail(f"version missing or dev (got {version!r})")

if "helixEnabled" not in data or data.get("helixEnabled") is None:
    fail("helixEnabled missing or null")
if not isinstance(data.get("helixEnabled"), bool):
    fail(f"helixEnabled must be boolean (got {type(data.get('helixEnabled')).__name__})")

routes = data.get("routes") or {}
if routes.get("vodHint") is not True:
    fail(f"routes.vodHint !== true (got {routes.get('vodHint')!r})")
if routes.get("backfill") is not True:
    fail(f"routes.backfill !== true (got {routes.get('backfill')!r})")

if expect_hosted and data.get("hostedMode") is not True:
    fail(f"hostedMode !== true (got {data.get('hostedMode')!r})")

for key in forbidden:
    if key in data:
        fail(f"public health exposes forbidden key {key!r}")

print("OK: pulse public health smoke passed")
print(json.dumps({
    "ok": data.get("ok"),
    "version": version,
    "helixEnabled": data.get("helixEnabled"),
    "hostedMode": data.get("hostedMode"),
    "routes": routes,
}, indent=2))
PY
