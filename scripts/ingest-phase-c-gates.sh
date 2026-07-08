#!/usr/bin/env bash
# Collect Phase C shadow validation gates. Run on VPS after shadow soak.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
# shellcheck source=scripts/lib/hosted-limits-guard-resolve.sh
source "${SCRIPT_DIR}/lib/hosted-limits-guard-resolve.sh"

ARTIFACT_DIR="${INGEST_SHADOW_ARTIFACT_DIR:-runtime/ingest-shadow}"
TOLERANCE="${1:-2}"
MIN_SAMPLES="${2:-1000}"
OUT_DIR="${3:-runtime/evidence/ingest-core-phase-c-gates-$(date -u +%Y%m%dT%H%M%SZ)}"
mkdir -p "${OUT_DIR}"

FAIL=0
note_fail() { echo "FAIL: $*"; FAIL=1; }
note_pass() { echo "PASS: $*"; }
note_warn() { echo "WARN: $*"; }

docker_env() {
  local container="$1"
  local var="$2"
  docker inspect "${container}" --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null \
    | grep -E "^${var}=" | head -1 | cut -d= -f2- || echo unset
}

REDIS="$(docker ps --format '{{.Names}}' 2>/dev/null | grep -E 'redis' | head -1 || true)"
redis_stats() {
  if [[ -n "${REDIS}" ]]; then
    docker exec "${REDIS}" redis-cli INFO stats
  elif command -v redis-cli >/dev/null 2>&1; then
    redis-cli INFO stats
  fi
}

echo "==> Phase C gates $(date -Is)" | tee "${OUT_DIR}/summary.txt"

# Limits guard — hard fail if uncapped
echo "==> hosted limits guard" | tee -a "${OUT_DIR}/summary.txt"
if run_hosted_limits_guard | tee "${OUT_DIR}/hosted-limits-guard.txt"; then
  note_pass "hosted limits guard"
else
  note_fail "hot container missing or uncapped"
fi

# Shadow compare
if [[ -f "${ARTIFACT_DIR}/latest.jsonl" ]]; then
  du -sh "${ARTIFACT_DIR}" | tee "${OUT_DIR}/artifact-disk.txt"
  ls -lh "${ARTIFACT_DIR}" | tee "${OUT_DIR}/artifact-listing.txt"
  export INGEST_SHADOW_ARTIFACT_DIR="${ARTIFACT_DIR}"
  bash "${SCRIPT_DIR}/ingest-shadow-inspect.sh" "${OUT_DIR}/shadow-inspect" | tee "${OUT_DIR}/shadow-inspect-summary.txt" || true
  if bash "${SCRIPT_DIR}/ingest-shadow-compare.sh" --closed-only --diagnose "${TOLERANCE}" "${MIN_SAMPLES}" | tee "${OUT_DIR}/shadow-compare.txt"; then
    note_pass "shadow compare (closed minutes) within tolerance"
  else
    note_fail "shadow compare (closed minutes) below tolerance or insufficient samples"
  fi
  bash "${SCRIPT_DIR}/ingest-shadow-mismatch-report.sh" "${OUT_DIR}/shadow-mismatch" | tee "${OUT_DIR}/shadow-mismatch-summary.txt" || true
  grep -E '^closed_chat_match_rate=|^closed_total_emote_match_rate=|^closed_both_chat_and_emote_match_rate=|^top_emote_key_mismatch_count=' \
    "${OUT_DIR}/shadow-compare.txt" | tee "${OUT_DIR}/shadow-parity-metrics.txt" || true
  # Open-minute skew is informational only — not a Phase C blocker
  open_mismatch="$(grep -E '^open_mismatch_count=' "${OUT_DIR}/shadow-compare.txt" 2>/dev/null | cut -d= -f2 || echo 0)"
  if [[ "${open_mismatch}" -gt 0 ]]; then
    note_warn "open_minute_mismatch_count=${open_mismatch} (excluded from gate; see shadow-inspect/)"
  fi
  # Hard cap guard: abort if latest.jsonl alone exceeds 512MiB
  sz="$(wc -c < "${ARTIFACT_DIR}/latest.jsonl" 2>/dev/null || echo 0)"
  if [[ "${sz}" -gt $((512*1024*1024)) ]]; then
    note_fail "latest.jsonl exceeds 512MiB — rotate or reduce allowlist before continuing"
  fi
else
  note_fail "shadow artifact missing: ${ARTIFACT_DIR}/latest.jsonl"
fi

# Prometheus (ingest metrics post-release)
PROM="${PROMETHEUS_URL:-http://127.0.0.1:9090}"
if curl -fsS "${PROM}/api/v1/status/config" >/dev/null 2>&1; then
  for q in \
    'rate(ingest_messages_dropped_total[5m])' \
    'ingest_active_collectors' \
    'ingest_desired_collectors' \
    'ingest_flush_queue_depth' \
    'histogram_quantile(0.95,rate(analytics_rollup_write_duration_seconds_bucket[5m]))'; do
    curl -sG "${PROM}/api/v1/query" --data-urlencode "query=${q}" > "${OUT_DIR}/prom-$(echo "${q}" | tr '/()[]:' '_____' | cut -c1-40).json" || true
  done
  drops="$(curl -sG "${PROM}/api/v1/query" --data-urlencode 'query=rate(ingest_messages_dropped_total[5m])' | jq -r '.data.result[0].value[1] // "absent"')"
  if [[ "${drops}" == "absent" || "${drops}" == "0" || "${drops}" == "0.0" ]]; then
    note_pass "ingest drop rate ${drops}"
  else
    note_fail "ingest drop rate ${drops}"
  fi
else
  note_warn "Prometheus unreachable — capture docker stats manually"
fi

# Redis rejected_connections delta vs baseline (operator compares files)
if stats="$(redis_stats 2>/dev/null)"; then
  echo "${stats}" | grep -E 'rejected_connections|total_connections_received' | tee "${OUT_DIR}/redis-stats.txt"
fi

# Public hub ingest block (shadow modes, legacy writer)
BASE="${PUBLIC_API_BASE:-https://api.streampulse.stream}"
curl -s "${BASE}/v1/public/hub?activityWindow=24h" | jq '.ingest, .coverage.state' | tee "${OUT_DIR}/hub-ingest.json"
core="$(curl -s "${BASE}/v1/public/hub?activityWindow=24h" | jq -r '.ingest.coreEnabled // "null"')"
if [[ "${core}" == "false" || "${core}" == "null" ]]; then
  note_pass "hub ingest.coreEnabled=${core} (legacy writer)"
else
  note_fail "hub ingest.coreEnabled=${core} — Phase C must stay false"
fi

# Phase C does not write PG — env proof on analytics container
ANALYTICS="$(docker ps --format '{{.Names}}' 2>/dev/null | grep -E 'analytics' | grep -v workers | head -1 || true)"
if [[ -n "${ANALYTICS}" ]]; then
  enabled="$(docker_env "${ANALYTICS}" INGEST_CORE_ENABLED)"
  dual="$(docker_env "${ANALYTICS}" INGEST_CORE_DUAL_READ_MODE)"
  shadow="$(docker_env "${ANALYTICS}" INGEST_CORE_SHADOW_MODE)"
  {
    echo "INGEST_CORE_ENABLED=${enabled}"
    echo "INGEST_CORE_DUAL_READ_MODE=${dual}"
    echo "INGEST_CORE_SHADOW_MODE=${shadow}"
  } | tee "${OUT_DIR}/ingest-env.txt"
  if [[ "${enabled}" == "0" || "${enabled}" == "false" || "${enabled}" == "unset" ]] && \
     [[ "${dual}" == "1" || "${dual}" == "true" ]] && \
     [[ "${shadow}" == "1" || "${shadow}" == "true" ]]; then
    note_pass "Phase C env: enabled=${enabled} dual=${dual} shadow=${shadow}"
  else
    note_fail "Phase C env mismatch enabled=${enabled} dual=${dual} shadow=${shadow}"
  fi
  docker logs "${ANALYTICS}" 2>&1 | grep -i 'ingest-core active' | tail -3 | tee "${OUT_DIR}/ingest-startup-log.txt" || note_warn "ingest-core startup log line not found"
fi

echo "---" | tee -a "${OUT_DIR}/summary.txt"
if [[ "${FAIL}" -eq 0 ]]; then
  echo "PHASE_C_GATES: PASS — recommend reviewing evidence before Phase D GO" | tee -a "${OUT_DIR}/summary.txt"
  echo "PHASE_D_GO_NOGO: GO pending operator sign-off" | tee -a "${OUT_DIR}/summary.txt"
  exit 0
fi
echo "PHASE_C_GATES: FAIL" | tee -a "${OUT_DIR}/summary.txt"
echo "PHASE_D_GO_NOGO: NO-GO" | tee -a "${OUT_DIR}/summary.txt"
exit 1
