#!/usr/bin/env bash
# Verify Phase C preconditions (read-only checks). Exit 1 on hard failures.
set -euo pipefail

BASE="${PUBLIC_API_BASE:-https://api.streampulse.stream}"
FAIL=0

docker_env() {
  local container="$1"
  local var="$2"
  docker inspect "${container}" --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null \
    | grep -E "^${var}=" | head -1 | cut -d= -f2- || echo unset
}

echo "==> Phase C preconditions check"

# 1. Moments Cache-Control
bucketT=$(( ($(date +%s)/300*300 - 600) * 1000 ))
headers="$(curl -sI "${BASE}/v1/public/hub/moments?bucketT=${bucketT}&activityWindow=1440")"
if echo "${headers}" | grep -qi '^HTTP/.* 200'; then
  if echo "${headers}" | grep -qi '^Cache-Control:'; then
    echo "PASS: moments Cache-Control present"
  else
    echo "FAIL: moments 200 but Cache-Control missing — deploy hub_historical_moments.go before shadow"
    FAIL=1
  fi
else
  echo "WARN: moments probe did not return 200 (bucketT=${bucketT})"
  FAIL=1
fi

# 2. Hub health
hub_code="$(curl -s -o /dev/null -w '%{http_code}' "${BASE}/v1/public/hub?activityWindow=24h")"
if [[ "${hub_code}" == "200" ]]; then
  echo "PASS: hub HTTP 200"
else
  echo "FAIL: hub HTTP ${hub_code}"
  FAIL=1
fi

# 3. INGEST_CORE_ENABLED default on running analytics (VPS only)
ANALYTICS="$(docker ps --format '{{.Names}}' 2>/dev/null | grep -E 'analytics' | grep -v workers | head -1 || true)"
env_val() {
  local var="$1"
  docker_env "${ANALYTICS}" "${var}"
}

if [[ -n "${ANALYTICS}" ]]; then
  core="$(env_val INGEST_CORE_ENABLED)"
  dual="$(env_val INGEST_CORE_DUAL_READ_MODE)"
  if [[ -z "${core}" || "${core}" == "0" || "${core}" == "false" || "${core}" == "unset" ]]; then
    echo "PASS: INGEST_CORE_ENABLED not cutover (${core:-unset})"
  else
    echo "FAIL: INGEST_CORE_ENABLED=${core} — must be 0 before Phase C shadow"
    FAIL=1
  fi
  corpus="$(env_val CORPUS_WORKERS_ENABLED)"
  scraper="$(env_val SCRAPER_ENABLED_ON_API_NODE)"
  if [[ "${corpus}" == "0" || "${corpus}" == "false" || "${corpus}" == "unset" ]]; then
    echo "PASS: CORPUS_WORKERS_ENABLED=${corpus}"
  else
    echo "WARN: CORPUS_WORKERS_ENABLED=${corpus} (expect 0 on API node)"
  fi
  if [[ "${scraper}" == "0" || "${scraper}" == "false" || "${scraper}" == "unset" ]]; then
    echo "PASS: SCRAPER_ENABLED_ON_API_NODE=${scraper}"
  else
    echo "WARN: SCRAPER_ENABLED_ON_API_NODE=${scraper} (expect 0 on API node)"
  fi
  if [[ "${dual}" == "1" || "${dual}" == "true" ]]; then
    echo "INFO: dual-read already enabled (${dual})"
  fi
else
  echo "SKIP: analytics container not reachable on this host — verify env via streampulse-ops overlay on VPS"
fi

# 4. Docker limits (informational — cannot auto-verify without compose inspect)
echo "INFO: confirm Docker mem_limit applied separately with 15–30m soak per service (hosted-resource-limits.compose.yml)"

if [[ "${FAIL}" -ne 0 ]]; then
  echo "PRECONDITIONS: FAIL"
  exit 1
fi
echo "PRECONDITIONS: PASS (operator must still confirm Docker limits + rollback anchors)"
exit 0
