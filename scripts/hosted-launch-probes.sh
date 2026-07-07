#!/usr/bin/env bash
set -euo pipefail

BASE="${PULSE_SMOKE_BASE_URL:-https://api.streampulse.stream}"
WINDOW="${PULSE_PROBE_ACTIVITY_WINDOW:-30m}"
MAX_METADATA_STALE_RATIO="${PULSE_PROBE_MAX_METADATA_STALE_RATIO:-0.25}"
MAX_OLDEST_QUEUED_SECONDS="${PULSE_PROBE_MAX_OLDEST_QUEUED_SECONDS:-172800}"
MAX_DEPLOY_DRIFT_DAYS="${PULSE_PROBE_MAX_DEPLOY_DRIFT_DAYS:-3}"
SSH_TARGET="${PULSE_PROBE_SSH_TARGET:-}"
REMOTE_APP="${PULSE_PROBE_REMOTE_APP:-/opt/streamclone/app}"
SSH_KEY="${PULSE_PROBE_SSH_KEY:-}"
OPS_LOCAL_BASE="${PULSE_OPS_LOCAL_BASE:-http://127.0.0.1:8090}"
OPS_TOKEN="${PULSE_OPS_PROBE_TOKEN:-}"
STRICT=0

fail=0
warn=0

usage() {
  cat <<'EOF'
Usage: bash scripts/hosted-launch-probes.sh [--strict]

Collects hosted launch evidence. By default continues after hub/readiness
failures so operators get a full checklist transcript. --strict exits on
first hub fetch/validation failure (CI mode).
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --strict) STRICT=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown arg: $1" >&2; usage; exit 2 ;;
  esac
done

echo "==> Hosted launch probes: ${BASE} activityWindow=${WINDOW} strict=${STRICT}"

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

fetch_json_with_header() {
  local url="$1"
  local out="$2"
  local header="$3"
  local code
  code="$(curl -fsS -H "${header}" -o "${out}" -w '%{http_code}' "${url}" || true)"
  if [[ "${code}" != "200" ]]; then
    echo "FAIL: GET ${url} HTTP ${code:-curl_error}" >&2
    return 1
  fi
  return 0
}

hub_tmp="$(mktemp)"
readiness_tmp="$(mktemp)"
trap 'rm -f "${hub_tmp}" "${readiness_tmp}"' EXIT

if ! fetch_json "${BASE}/v1/public/hub?activityWindow=${WINDOW}" "${hub_tmp}"; then
  fail=1
  if [[ "${STRICT}" -eq 1 ]]; then
    exit 1
  fi
else
  if ! python3 - "${hub_tmp}" "${MAX_METADATA_STALE_RATIO}" "${MAX_OLDEST_QUEUED_SECONDS}" <<'PY'
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
    failures.append("live admission is enabled with live roster rows but collectorMax is 0")
if live > 0 and live_admission and collector_active <= 0 and collector_tracking <= 0:
    failures.append("live admission is enabled with live roster rows but no active/tracking collector rows")
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
for tier_name in ("gold", "silver"):
    tier = cp.get(tier_name) or {}
    if tier:
        print("{tier}: queued={queued} running={running} failed={failed} oldestQueuedSeconds={oldest}".format(
            tier=tier_name,
            queued=tier.get("queued"),
            running=tier.get("running"),
            failed=tier.get("failed"),
            oldest=tier.get("oldestQueuedSeconds"),
        ))
if failures:
    for failure in failures:
        print(f"FAIL: {failure}", file=sys.stderr)
    sys.exit(1)
PY
  then
    fail=1
    if [[ "${STRICT}" -eq 1 ]]; then
      exit 1
    fi
  fi
fi

readiness_ok=0
if [[ -n "${SSH_TARGET}" ]]; then
  ssh_args=(-o BatchMode=yes -o StrictHostKeyChecking=accept-new)
  if [[ -n "${SSH_KEY}" ]]; then
    ssh_args+=(-i "${SSH_KEY}")
  fi
  remote_readiness="set -euo pipefail; token=\$(grep -E '^PULSE_OPS_PROBE_TOKEN=' private-host-env-file 2>/dev/null | cut -d= -f2- || true); if [[ -z \"\${token}\" ]]; then token=\${PULSE_OPS_PROBE_TOKEN:-}; fi; code=\$(curl -sS -o /tmp/sp-readiness.json -w '%{http_code}' -H \"X-Ops-Probe-Token: \${token}\" '${OPS_LOCAL_BASE}/v1/internal/ops/readiness?topN=500'); if [[ \"\${code}\" != \"200\" ]]; then echo \"FAIL: internal ops readiness HTTP \${code}\" >&2; exit 1; fi; cat /tmp/sp-readiness.json"
  if ssh "${ssh_args[@]}" "${SSH_TARGET}" "${remote_readiness}" >"${readiness_tmp}"; then
    readiness_ok=1
  else
    echo "FAIL: internal ops readiness via SSH failed" >&2
    fail=1
  fi
elif [[ -n "${OPS_TOKEN}" ]]; then
  if fetch_json_with_header "${OPS_LOCAL_BASE}/v1/internal/ops/readiness?topN=500" "${readiness_tmp}" "X-Ops-Probe-Token: ${OPS_TOKEN}"; then
    readiness_ok=1
  else
    fail=1
  fi
else
  echo "WARN: set PULSE_PROBE_SSH_TARGET or PULSE_OPS_PROBE_TOKEN for internal readiness" >&2
  warn=1
fi

if [[ "${readiness_ok}" -eq 1 ]]; then
  if ! python3 - "${readiness_tmp}" <<'PY'
import json
import sys
with open(sys.argv[1], "r", encoding="utf-8") as fh:
    data = json.load(fh)
summary = data.get("summary") or {}
print("readiness: admissionEnabled={admission} liveRows={live} collectorMax={collector} metadataStaleRows={stale}".format(
    admission=data.get("admissionEnabled"),
    live=summary.get("liveRows"),
    collector=data.get("collectorMax"),
    stale=summary.get("metadataStaleRows"),
))
PY
  then
    fail=1
  fi
fi

if [[ -n "${SSH_TARGET}" ]]; then
  ssh_args=(-o BatchMode=yes -o StrictHostKeyChecking=accept-new)
  if [[ -n "${SSH_KEY}" ]]; then
    ssh_args+=(-i "${SSH_KEY}")
  fi
  remote_cmd="set -e; cd '${REMOTE_APP}'; deployed=\$(cat DEPLOYED_SHA 2>/dev/null || true); head=\$(git rev-parse HEAD 2>/dev/null || true); age=\$(find DEPLOYED_SHA -maxdepth 0 -mtime +${MAX_DEPLOY_DRIFT_DAYS} -print 2>/dev/null || true); cloud=\$(systemctl is-active cloudflared 2>/dev/null || true); stream_ver=\$(docker exec \$(docker ps --format '{{.Names}}' | grep analytics | head -n1) printenv STREAMCLONE_VERSION 2>/dev/null || true); printf 'remote: deployed=%s head=%s cloudflared=%s streamclone_version=%s\\n' \"\${deployed}\" \"\${head}\" \"\${cloud}\" \"\${stream_ver}\"; test -z \"\${age}\"; test \"\${cloud}\" = active"
  if ! ssh "${ssh_args[@]}" "${SSH_TARGET}" "${remote_cmd}"; then
    echo "FAIL: remote deployed SHA/cloudflared probe failed" >&2
    fail=1
  fi
else
  echo "WARN: PULSE_PROBE_SSH_TARGET unset; skipping DEPLOYED_SHA and cloudflared systemd checks" >&2
  warn=1
fi

if [[ "${fail}" -ne 0 ]]; then
  exit 1
fi
if [[ "${warn}" -ne 0 ]]; then
  echo "hosted launch probes completed with warnings"
else
  echo "hosted launch probes OK"
fi
