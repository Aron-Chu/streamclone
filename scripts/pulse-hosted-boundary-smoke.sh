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
    echo "FAIL: jq is required for pulse-hosted-boundary-smoke (install jq or use BearHost VPS)" >&2
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
  else
    echo "SKIP: DB fixture lookup (hosted SSH fixtures moved to streampulse-ops)"
  fi
  FIXTURE_SOURCE=fallback
  return 0
}

curl_code() {
  curl -sS -o /dev/null -w '%{http_code}' "$@"
}

curl_body() {
  local out="$1"
  shift
  curl -sS -o "${out}" "$@"
}

expect_blocked() {
  local method="$1"
  local path="$2"
  local code
  code="$(curl -sS -o /dev/null -w '%{http_code}' -X "${method}" "${BASE}${path}")"
  if [[ "${code}" != "404" && "${code}" != "403" ]]; then
    echo "FAIL: ${method} ${path} HTTP ${code} (want 404/403 blocked)" >&2
    return 1
  fi
  echo "OK: ${method} ${path} HTTP ${code} (blocked)"
  return 0
}

expect_auth_required() {
  local method="$1"
  local path="$2"
  local code
  code="$(curl -sS -o /dev/null -w '%{http_code}' -X "${method}" "${BASE}${path}")"
  if [[ "${code}" != "401" && "${code}" != "403" ]]; then
    echo "FAIL: ${method} ${path} HTTP ${code} (want 401/403 auth required)" >&2
    return 1
  fi
  echo "OK: ${method} ${path} HTTP ${code} (auth required)"
  return 0
}

expect_404() {
  local method="$1"
  local path="$2"
  local code
  code="$(curl -sS -o /dev/null -w '%{http_code}' -X "${method}" "${BASE}${path}")"
  if [[ "${code}" != "404" ]]; then
    echo "FAIL: ${method} ${path} HTTP ${code} (want 404)" >&2
    return 1
  fi
  echo "OK: ${method} ${path} HTTP 404"
  return 0
}

expect_admin_auth_no_operator_leak() {
  local path="$1"
  local tmp code
  tmp="$(mktemp)"
  code="$(curl -sS -o "${tmp}" -w '%{http_code}' "${BASE}${path}")"
  if [[ "${code}" != "401" && "${code}" != "403" ]]; then
    echo "FAIL: GET ${path} HTTP ${code} (want 401/403 admin auth required)" >&2
    rm -f "${tmp}"
    return 1
  fi
  if grep -Eq '"(caps|rates|queues|config|trackedChannels|alwaysTracked|jobs|rateLimitKeysSampled|watchRateLimitPerMin|backfillRateLimitPerHour)"' "${tmp}"; then
    echo "FAIL: GET ${path} leaked admin operator fields while unauthenticated" >&2
    rm -f "${tmp}"
    return 1
  fi
  rm -f "${tmp}"
  echo "OK: GET ${path} HTTP ${code} (admin auth required, no operator fields)"
  return 0
}

expect_not_blocked() {
  local method="$1"
  local path="$2"
  local code
  code="$(curl -sS -o /dev/null -w '%{http_code}' -X "${method}" "${BASE}${path}")"
  if [[ "${code}" == "404" ]]; then
    echo "FAIL: ${method} ${path} HTTP 404 (must remain reachable)" >&2
    return 1
  fi
  echo "OK: ${method} ${path} HTTP ${code} (not blocked)"
  return 0
}

phase_a_public_boundary() {
  local fail=0
  local code health_tmp

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
    "/v1/analytics/streams/${STREAM_ID}"; do
    code="$(curl_code "${BASE}${path}")"
    if [[ "${code}" != "401" ]]; then
      echo "FAIL: ${path} HTTP ${code} (want 401 unauthenticated raw analytics)" >&2
      fail=1
    else
      echo "OK: ${path} HTTP 401"
    fi
  done

  # Sanitized portal BFF routes are public-safe (no auth); see portal_analytics_api_test.go.
  for path in \
    "/v1/portal/analytics/channels/ludwig/live" \
    "/v1/portal/analytics/channels/ludwig/streams"; do
    code="$(curl_code "${BASE}${path}")"
    if [[ "${code}" != "200" ]]; then
      echo "FAIL: ${path} HTTP ${code} (want 200 public-safe portal JSON)" >&2
      fail=1
    else
      echo "OK: ${path} HTTP 200 (portal public-safe)"
    fi
  done

  # Public emotes overview remains live; atlas route retirement is deferred (P2-021 policy).
  local emotes_tmp
  emotes_tmp="$(mktemp)"
  code="$(curl_code "${BASE}/v1/public/emotes/overview?range=7d")"
  if [[ "${code}" != "200" ]]; then
    echo "FAIL: /v1/public/emotes/overview HTTP ${code} (want 200 public overview)" >&2
    fail=1
  else
    curl_body "${emotes_tmp}" "${BASE}/v1/public/emotes/overview?range=7d"
    if ! jq -e '.range == "7d"' "${emotes_tmp}" >/dev/null 2>&1; then
      echo "FAIL: /v1/public/emotes/overview payload missing range=7d" >&2
      fail=1
    else
      echo "OK: /v1/public/emotes/overview HTTP 200 (range=7d)"
    fi
  fi
  rm -f "${emotes_tmp}"

  echo "==> Phase A: blocked hosted edge routes"
  local blocked_fail=0
  local method path
  while IFS=$'\t' read -r method path; do
    [[ -z "${method}" ]] && continue
    if ! expect_blocked "${method}" "${path}"; then
      blocked_fail=1
    fi
  done <<'BLOCKED_ROUTES'
GET	/v1/analytics/streams/999999/chat-replay?limit=1
GET	/v1/analytics/streams/999999/chat-replay/
GET	/v1/analytics/streams/999999/chat-messages
GET	/v1/analytics/channels/ludwig/chat-logs/messages
DELETE	/v1/analytics/streams/999999/chat-messages
POST	/v1/analytics/chat/ingest
GET	/v1/analytics/tracking/snapshot
GET	/v1/analytics/top100/readiness
GET	/v1/analytics/top-roster/readiness?topN=500
GET	/v1/corpus/readiness
GET	/v1/internal/corpus/readiness
GET	/metrics
GET	/metrics/
POST	/v1/analytics/streams/999999/sync
GET	/v1/analytics/sync/active
POST	/v1/analytics/streams/999999/prefetch-tracker
GET	/v1/analytics/streams/999999/sync/status
POST	/v1/analytics/streams/999999/sync/status
BLOCKED_ROUTES
  if [[ "${blocked_fail}" -ne 0 ]]; then
    fail=1
  fi

  echo "==> Phase A: auth-gated hosted ops routes"
  if ! expect_auth_required GET '/v1/internal/corpus/gaps?limit=1'; then
    fail=1
  fi
  if ! expect_admin_auth_no_operator_leak /v1/admin/pulse/health; then
    fail=1
  fi
  if ! expect_admin_auth_no_operator_leak /v1/admin/pulse/registry; then
    fail=1
  fi

  echo "==> Phase A: unmatched hosted edge routes"
  if ! expect_404 GET /grafana/; then
    fail=1
  fi
  if ! expect_404 GET /__pulse_smoke_unmatched__; then
    fail=1
  fi
  if ! expect_404 GET /v1/__pulse_smoke_unmatched__; then
    fail=1
  fi
  if ! expect_404 GET /v1/public/__pulse_smoke_unmatched__; then
    fail=1
  fi

  echo "==> Phase A: intentional public routes (must not be 404)"
  if ! expect_not_blocked GET /healthz; then
    fail=1
  fi
  if ! expect_not_blocked GET '/v1/public/hub?activityWindow=30m'; then
    fail=1
  fi
  if ! expect_not_blocked POST /v1/analytics/channels/ludwig/watch; then
    fail=1
  fi

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
