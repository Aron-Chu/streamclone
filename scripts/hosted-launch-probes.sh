#!/usr/bin/env bash
# Public-API-only hosted launch probes. Operator SSH/internal ops live in private streampulse-ops.
set -euo pipefail

BASE="${PULSE_SMOKE_BASE_URL:-https://api.streampulse.stream}"
WINDOW="${PULSE_PROBE_ACTIVITY_WINDOW:-30m}"
MAX_METADATA_STALE_RATIO="${PULSE_PROBE_MAX_METADATA_STALE_RATIO:-0.25}"
MAX_OLDEST_QUEUED_SECONDS="${PULSE_PROBE_MAX_OLDEST_QUEUED_SECONDS:-172800}"

echo "==> Public hosted launch probes: ${BASE} activityWindow=${WINDOW}"

fetch_json() {
  local url="$1"
  local out="$2"
  local code
  code="$(curl -fsS -o "${out}" -w '%{http_code}' "${url}" || true)"
  if [[ "${code}" != "200" ]]; then
    echo "FAIL: GET ${url} HTTP ${code:-curl_error}" >&2
    return 1
  fi
  return 0
}

hub_tmp="$(mktemp)"
health_tmp="$(mktemp)"
trap 'rm -f "${hub_tmp}" "${health_tmp}"' EXIT

if ! fetch_json "${BASE}/v1/public/hub?activityWindow=${WINDOW}" "${hub_tmp}"; then
  exit 1
fi

python3 - "${hub_tmp}" "${MAX_METADATA_STALE_RATIO}" "${MAX_OLDEST_QUEUED_SECONDS}" <<'PY'
import json
import sys

path, max_stale_ratio_raw, max_oldest_raw = sys.argv[1:4]
max_stale_ratio = float(max_stale_ratio_raw)
max_oldest = int(max_oldest_raw)
with open(path, "r", encoding="utf-8") as fh:
    hub = json.load(fh)
cp = hub.get("corpusPipeline") or {}
roster = cp.get("roster") or {}
coverage = hub.get("coverage") or {}
failures = []

live = int(roster.get("live") or 0)
metadata_stale = int(roster.get("metadataStale") or 0)
live_admission = bool(cp.get("liveAdmissionEnabled"))
collector_max = int(cp.get("collectorMax") or 0)
collector_active = int(cp.get("collectorActive") or 0)
collector_tracking = int(roster.get("collectorTracking") or 0)

def oldest_seconds(tier_name):
    tier = cp.get(tier_name) or {}
    value = tier.get("oldestQueuedSeconds")
    return int(value) if isinstance(value, (int, float)) else None

if live > 0 and live_admission and collector_max <= 0:
    failures.append("live admission enabled with live roster rows but collectorMax is 0")
if live > 0 and live_admission and collector_active <= 0 and collector_tracking <= 0:
    failures.append("live admission enabled with live roster rows but no active/tracking collector rows")
if live > 0 and metadata_stale / live > max_stale_ratio:
    failures.append(f"metadata stale ratio {metadata_stale}/{live} exceeds {max_stale_ratio:.2f}")
for tier_name in ("gold", "silver"):
    oldest = oldest_seconds(tier_name)
    if oldest is not None and oldest > max_oldest:
        failures.append(f"{tier_name} oldestQueuedSeconds {oldest} exceeds {max_oldest}")

print("hub: state={state} liveAdmission={admission} collector={active}/{max} tracking={tracking} live={live} metadataStale={stale}".format(
    state=coverage.get("state"),
    admission=live_admission,
    active=collector_active,
    max=collector_max,
    tracking=collector_tracking,
    live=live,
    stale=metadata_stale,
))
if failures:
    for failure in failures:
        print(f"FAIL: {failure}", file=sys.stderr)
    sys.exit(1)
PY

if ! fetch_json "${BASE}/v1/extension/health" "${health_tmp}"; then
  exit 1
fi

python3 - "${health_tmp}" <<'PY'
import json
import sys
with open(sys.argv[1], "r", encoding="utf-8") as fh:
    data = json.load(fh)
version = data.get("version") or data.get("streamcloneVersion") or "unknown"
print(f"health: version={version}")
PY

echo "public hosted launch probes OK"
