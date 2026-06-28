#!/usr/bin/env bash
# Post-deploy hosted boundary + chart/VOD canary smoke (read-only HTTP; optional SSH fixtures).
#
# Usage:
#   bash scripts/pulse-hosted-boundary-smoke.sh
#   PULSE_BETA_KEY=... bash scripts/pulse-hosted-boundary-smoke.sh
#
# Env:
#   PULSE_SMOKE_BASE_URL     default https://api.streampulse.stream
#   PULSE_SMOKE_STREAM_ID    override chart canary stream (fallback 316860077047)
#   PULSE_SMOKE_VOD_ID       override VOD canary (fallback 2804592918)
#   PULSE_EXPECT_VERSION     optional health version substring match
#   PULSE_SMOKE_SKIP_SSH     set 1 to skip DB fixture lookup via BearHost SSH
set -euo pipefail

require_jq() {
  if ! command -v jq >/dev/null 2>&1; then
    echo "FAIL: jq is required for pulse-hosted-boundary-smoke (install jq or use legacy-rollback-host)" >&2
    exit 1
  fi
}

require_jq

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE="${PULSE_SMOKE_BASE_URL:-https://api.streampulse.stream}"
BASE="${BASE%/}"
BETA_KEY="${PULSE_BETA_KEY:-}"
if [[ -z "${BETA_KEY}" && -n "${PULSE_BETA_KEYS:-}" ]]; then
  BETA_KEY="${PULSE_BETA_KEYS%%,*}"
fi
STREAM_ID="${PULSE_SMOKE_STREAM_ID:-316860077047}"
VOD_ID="${PULSE_SMOKE_VOD_ID:-2804592918}"
STREAM_ROLLUPS="${PULSE_SMOKE_STREAM_ROLLUPS:-0}"
VOD_ROLLUPS="${PULSE_SMOKE_VOD_ROLLUPS:-0}"

PUBLIC_BOUNDARY=FAIL
CHART_CANARY=SKIP
VOD_EXTENSION_CANARY=SKIP
FIXTURE_SOURCE=fallback

load_db_fixtures() {
  if [[ "${PULSE_SMOKE_SKIP_SSH:-}" == "1" ]]; then
    echo "SKIP: DB fixture lookup (PULSE_SMOKE_SKIP_SSH=1)"
    FIXTURE_SOURCE=fallback
    return 0
  fi
  if [[ ! -f "${ROOT}/scripts/lib/bearhost-ssh.sh" ]]; then
    echo "WARN: bearhost-ssh unavailable; chart/VOD canaries will SKIP" >&2
    FIXTURE_SOURCE=fallback
    return 0
  fi
  # shellcheck source=scripts/lib/bearhost-ssh.sh
  source "${ROOT}/scripts/lib/bearhost-ssh.sh"
  bearhost_ssh_config
  if [[ ! -f "${BEARHOST_SSH_KEY:-}" ]]; then
    echo "WARN: BearHost SSH key missing; chart/VOD canaries will SKIP" >&2
    FIXTURE_SOURCE=fallback
    return 0
  fi
  local fixture_out
  fixture_out="$(bearhost_ssh "cd '${BEARHOST_REMOTE_APP}' && bash scripts/lib/bearhost-smoke-fixtures-remote.sh" 2>/dev/null || true)"
  if [[ -z "${fixture_out}" ]]; then
    echo "WARN: DB fixture lookup failed; chart/VOD canaries will SKIP" >&2
    FIXTURE_SOURCE=fallback
    return 0
  fi
  FIXTURE_SOURCE=db
  while IFS= read -r line; do
    case "${line}" in
      PULSE_SMOKE_STREAM_ID=*) STREAM_ID="${line#PULSE_SMOKE_STREAM_ID=}" ;;
      PULSE_SMOKE_STREAM_ROLLUPS=*) STREAM_ROLLUPS="${line#PULSE_SMOKE_STREAM_ROLLUPS=}" ;;
      PULSE_SMOKE_VOD_ID=*) VOD_ID="${line#PULSE_SMOKE_VOD_ID=}" ;;
      PULSE_SMOKE_VOD_ROLLUPS=*) VOD_ROLLUPS="${line#PULSE_SMOKE_VOD_ROLLUPS=}" ;;
    esac
  done <<< "${fixture_out}"
  echo "==> DB-backed fixtures: stream_id=${STREAM_ID} rollups=${STREAM_ROLLUPS} vod_id=${VOD_ID} vod_rollups=${VOD_ROLLUPS}"
}

curl_code() {
  curl -sS -o /dev/null -w '%{http_code}' "$@"
}

curl_body() {
  local out="$1"
  shift
  curl -sS -o "${out}" "$@"
}

phase_a_public_boundary() {
  local fail=0
  local code health_tmp emotes_tmp

  echo "==> Phase A: public boundary (${BASE})"

  health_tmp="$(mktemp)"
  code="$(curl_code "${BASE}/v1/extension/health")"
  if [[ "${code}" != "200" ]]; then
    echo "FAIL: /v1/extension/health HTTP ${code} (want 200)" >&2
    fail=1
  else
    curl_body "${health_tmp}" "${BASE}/v1/extension/health"
    if ! grep -q '"ok":true' "${health_tmp}" 2>/dev/null; then
      echo "FAIL: health payload missing ok:true" >&2
      fail=1
    fi
    if [[ -n "${PULSE_EXPECT_VERSION:-}" ]] && ! grep -q "${PULSE_EXPECT_VERSION}" "${health_tmp}" 2>/dev/null; then
      echo "FAIL: health version missing expected ${PULSE_EXPECT_VERSION}" >&2
      fail=1
    fi
    echo "OK: /v1/extension/health HTTP 200"
  fi
  rm -f "${health_tmp}"

  for path in \
    "/v1/analytics/channels/ludwig/live" \
    "/v1/analytics/channels/ludwig/live?sparse=false" \
    "/v1/analytics/streams/${STREAM_ID}" \
    "/v1/portal/analytics/channels/ludwig/live" \
    "/v1/portal/analytics/channels/ludwig/streams"; do
    code="$(curl_code "${BASE}${path}")"
    if [[ "${code}" != "401" ]]; then
      echo "FAIL: ${path} HTTP ${code} (want 401 unauthenticated)" >&2
      fail=1
    else
      echo "OK: ${path} HTTP 401"
    fi
  done

  emotes_tmp="$(mktemp)"
  code="$(curl -sS -o "${emotes_tmp}" -w '%{http_code}' "${BASE}/v1/public/emotes/overview?range=7d")"
  if [[ "${code}" == "404" ]]; then
    echo "FAIL: /v1/public/emotes/overview HTTP 404 (route not deployed)" >&2
    fail=1
  elif [[ "${code}" == "200" || "${code}" == "503" ]]; then
    if ! jq -e 'type == "object" and (.aggregateOnly != null or .state != null or .schemaVersion != null or .error != null or .unavailableReason != null)' "${emotes_tmp}" >/dev/null 2>&1; then
      echo "FAIL: /v1/public/emotes/overview HTTP ${code} but not valid JSON contract" >&2
      fail=1
    else
      echo "OK: /v1/public/emotes/overview HTTP ${code} (not 404)"
    fi
  else
    echo "FAIL: /v1/public/emotes/overview HTTP ${code} (want 200 or 503 JSON)" >&2
    fail=1
  fi
  rm -f "${emotes_tmp}"

  if [[ "${fail}" -eq 0 ]]; then
    PUBLIC_BOUNDARY=PASS
  fi
}

phase_b_chart_canary() {
  if [[ -z "${BETA_KEY}" ]]; then
    echo "==> Phase B: chart canary SKIP (set PULSE_BETA_KEY)"
    CHART_CANARY=SKIP
    return 0
  fi

  if [[ "${FIXTURE_SOURCE}" != "db" ]]; then
    echo "==> Phase B: chart canary SKIP (FIXTURE_SOURCE=${FIXTURE_SOURCE}; DB-backed proof required)"
    CHART_CANARY=SKIP
    return 0
  fi

  echo "==> Phase B: chart canary (stream_id=${STREAM_ID}, rollups=${STREAM_ROLLUPS})"
  local fail=0 minutes_tmp detail_tmp code minute_count

  minutes_tmp="$(mktemp)"
  code="$(curl -sS -o "${minutes_tmp}" -w '%{http_code}' \
    -H "X-Streamclone-Beta-Key: ${BETA_KEY}" \
    "${BASE}/v1/portal/analytics/streams/${STREAM_ID}/minutes")"
  if [[ "${code}" != "200" ]]; then
    echo "FAIL: portal minutes HTTP ${code} (want 200)" >&2
    fail=1
  else
    minute_count="$(jq '.minutes | length' "${minutes_tmp}")"
    if [[ "${STREAM_ROLLUPS}" -gt 0 && "${minute_count}" -le 0 ]]; then
      echo "FAIL: portal minutes empty but DB rollups=${STREAM_ROLLUPS}" >&2
      fail=1
    elif [[ "${STREAM_ROLLUPS}" -le 0 ]]; then
      echo "FAIL: DB fixture has zero rollups; cannot prove chart canary" >&2
      fail=1
    else
      echo "OK: portal minutes HTTP 200 count=${minute_count}"
    fi
  fi
  rm -f "${minutes_tmp}"

  detail_tmp="$(mktemp)"
  code="$(curl -sS -o "${detail_tmp}" -w '%{http_code}' \
    -H "X-Streamclone-Beta-Key: ${BETA_KEY}" \
    "${BASE}/v1/analytics/streams/${STREAM_ID}?sparse=true")"
  if [[ "${code}" != "200" ]]; then
    echo "FAIL: authorized stream detail HTTP ${code} (want 200)" >&2
    fail=1
  else
    if ! jq -e '.rollups' "${detail_tmp}" >/dev/null 2>&1; then
      echo "FAIL: authorized stream detail missing rollups" >&2
      fail=1
    else
      echo "OK: authorized stream detail HTTP 200 with rollups"
    fi
  fi
  rm -f "${detail_tmp}"

  if [[ "${fail}" -eq 0 ]]; then
    CHART_CANARY=PASS
  else
    CHART_CANARY=FAIL
  fi
}

phase_c_vod_canary() {
  if [[ "${FIXTURE_SOURCE}" != "db" ]]; then
    echo "==> Phase C: VOD extension canary SKIP (FIXTURE_SOURCE=${FIXTURE_SOURCE}; DB-backed proof required)"
    VOD_EXTENSION_CANARY=SKIP
    return 0
  fi

  echo "==> Phase C: VOD extension canary (vod_id=${VOD_ID}, rollups=${VOD_ROLLUPS})"
  local fail=0 vod_tmp code

  vod_tmp="$(mktemp)"
  code="$(curl -sS -o "${vod_tmp}" -w '%{http_code}' "${BASE}/v1/extension/pulse/vods/${VOD_ID}")"
  if [[ "${code}" == "404" ]]; then
    echo "FAIL: /v1/extension/pulse/vods/${VOD_ID} HTTP 404 (route or VOD not indexed)" >&2
    fail=1
  elif [[ "${code}" != "200" ]]; then
    echo "FAIL: VOD pulse HTTP ${code} (want 200 JSON)" >&2
    fail=1
  else
    if ! jq -e --argjson vod_rollups "${VOD_ROLLUPS}" '
      if ($vod_rollups | tonumber) > 0 then
        (.coverageStatus | IN("ready","partial","syncing"))
        and ((.timeline.points // []) | length) > 0
      else
        true
      end
    ' "${vod_tmp}" >/dev/null 2>&1; then
      echo "FAIL: VOD pulse missing coverage/timeline for vod with rollups=${VOD_ROLLUPS}" >&2
      fail=1
    else
      echo "OK: VOD pulse HTTP 200"
    fi
  fi
  rm -f "${vod_tmp}"

  if [[ "${fail}" -eq 0 ]]; then
    VOD_EXTENSION_CANARY=PASS
  else
    VOD_EXTENSION_CANARY=FAIL
  fi
}

portal_path_guard() {
  echo "==> portal path guard (streampulse-web)"
  if [[ -d "${ROOT}/../streamclone-pulse/streampulse-web/src" ]]; then
    if rg -q 'getAnalyticsStream|/v1/analytics/streams/' "${ROOT}/../streamclone-pulse/streampulse-web/src/lib/streamcloneAnalytics.ts" 2>/dev/null; then
      echo "WARN: streamcloneAnalytics still references /v1/analytics/streams (portal uses gated client)"
    fi
    if rg -q '/v1/analytics/streams' "${ROOT}/../streamclone-pulse/streampulse-web/src" --glob '!**/streamcloneAnalytics.ts' 2>/dev/null; then
      echo "FAIL: streampulse-web/src uses raw /v1/analytics/streams outside gated client" >&2
      exit 1
    fi
    echo "OK: streampulse-web/src has no stray raw analytics stream routes"
  fi
}

echo "==> Pulse hosted boundary smoke: ${BASE}"
load_db_fixtures
phase_a_public_boundary
phase_b_chart_canary
phase_c_vod_canary
portal_path_guard

echo ""
echo "FIXTURE_SOURCE=${FIXTURE_SOURCE}"
echo "PUBLIC_BOUNDARY=${PUBLIC_BOUNDARY}"
echo "CHART_CANARY=${CHART_CANARY}"
echo "VOD_EXTENSION_CANARY=${VOD_EXTENSION_CANARY}"

if [[ "${PUBLIC_BOUNDARY}" != "PASS" ]]; then
  exit 1
fi
if [[ "${CHART_CANARY}" == "FAIL" || "${VOD_EXTENSION_CANARY}" == "FAIL" ]]; then
  exit 1
fi

echo "OK: pulse-hosted-boundary-smoke passed"
