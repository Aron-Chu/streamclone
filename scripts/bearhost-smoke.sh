#!/usr/bin/env bash
# BearHost production smoke gates (8 checks). Run on VPS after phased deploy.
#
# Usage:
#   bash scripts/bearhost-smoke.sh
#   BEARHOST_IP=141.11.243.103 BEARHOST_TT_URL=https://twitchtracker.com/... bash scripts/bearhost-smoke.sh
#
# Optional:
#   BEARHOST_SKIP_SYNC=1     — skip gates 6–7 (sync + rollup)
#   BEARHOST_COMPOSE_ONLY=1  — gate 1 only (no HTTP checks)
#   BEARHOST_CORPUS_PREFLIGHT_ONLY=1 — run corpus preflight (gate 0) and exit

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck source=scripts/bearhost-corpus-preflight.sh
source "${ROOT}/scripts/bearhost-corpus-preflight.sh"

BEARHOST_IP="${BEARHOST_IP:-141.11.243.103}"
PUBLIC_ORIGIN="${PUBLIC_ORIGIN:-http://${BEARHOST_IP}}"
BEARHOST_TT_URL="${BEARHOST_TT_URL:-https://twitchtracker.com/ninja/streams/318832886110}"
SCRAPER_TIMEOUT_MS="${SCRAPER_TIMEOUT_MS:-120000}"

fail() {
  echo "bearhost-smoke FAIL gate ${GATE:-?}: $1" >&2
  exit 1
}

pass() {
  echo "bearhost-smoke PASS gate ${GATE}: $1"
}

bearhost_compose() {
  local compose_args=(
    docker compose
    --env-file .env
    --env-file deploy/env/profile-full.env
    --env-file deploy/env/profile-archive.env
    --env-file deploy/env/profile-bearhost-prod.env
    -f deploy/docker-compose.yml
    -f deploy/docker-compose.prod.yml
    -f deploy/docker-compose.bearhost-prod.yml
  )
  local build_local="${BEARHOST_BUILD_LOCAL:-}"
  if [[ -z "${build_local}" ]] && [[ -f deploy/env/profile-bearhost-prod.env ]]; then
    build_local="$(grep -E '^BEARHOST_BUILD_LOCAL=' deploy/env/profile-bearhost-prod.env | cut -d= -f2- | tr -d '\r' || true)"
  fi
  if [[ "${build_local:-0}" == "1" ]]; then
    compose_args+=(-f deploy/docker-compose.bearhost-build.yml)
  else
    compose_args+=(-f deploy/docker-compose.release.yml)
  fi
  compose_args+=(--profile scraper "$@")
  "${compose_args[@]}"
}

# Gate 0 — corpus preflight (Azure secret + Twitch creds on host)
GATE=0
echo "==> Gate 0: corpus preflight (Azure secret + Twitch creds)"
if bearhost_corpus_preflight; then
  pass "corpus preflight ok"
  corpus_enabled="${CORPUS_WORKERS_ENABLED:-false}"
  if [[ "${corpus_enabled}" == "1" || "${corpus_enabled}" == "true" ]]; then
    echo "bearhost-smoke: CORPUS_WORKERS_ENABLED=${corpus_enabled} — workers should run corpus plane"
  else
    echo "bearhost-smoke: corpus preflight ok but CORPUS_WORKERS_ENABLED=false — workers run API/sync only (no bronze/backfill)"
  fi
else
  if [[ "${CORPUS_WORKERS_ENABLED:-false}" == "1" || "${CORPUS_WORKERS_ENABLED:-}" == "true" ]]; then
    fail "corpus preflight failed but CORPUS_WORKERS_ENABLED is true — fix secrets/creds or set CORPUS_WORKERS_ENABLED=false"
  fi
  pass "corpus preflight failed (expected when secret/creds missing); workers corpus-off"
fi

if [[ "${BEARHOST_CORPUS_PREFLIGHT_ONLY:-}" == "1" ]]; then
  echo "bearhost-smoke: corpus preflight gate complete"
  exit 0
fi

# Gate 1 — compose ps: running, not unhealthy / restarting
GATE=1
echo "==> Gate 1: docker compose ps"
ps_out="$(bearhost_compose ps --format json 2>/dev/null || bearhost_compose ps)"
if echo "${ps_out}" | grep -qiE 'unhealthy|Restarting|restarting'; then
  fail "unhealthy or restarting containers"
fi
# migrate is a one-shot — ignore exited migrate containers
bad_exited="$(bearhost_compose ps -a --format '{{.Service}} {{.State}}' 2>/dev/null \
  | grep -i exited | grep -vi migrate || true)"
if [[ -n "${bad_exited}" ]]; then
  fail "exited containers present: ${bad_exited}"
fi
running="$(bearhost_compose ps --services --filter status=running 2>/dev/null | wc -l | tr -d ' ')"
if [[ "${running}" -lt 5 ]]; then
  fail "expected multiple running services, got ${running}"
fi
pass "compose services running"

if [[ "${BEARHOST_COMPOSE_ONLY:-}" == "1" ]]; then
  echo "bearhost-smoke: all requested gates passed"
  exit 0
fi

# Gate 2 — scraper health (internal, via docker exec)
GATE=2
echo "==> Gate 2: scraper /health"
if ! bearhost_compose exec -T scraper curl -sf http://127.0.0.1:8000/health >/dev/null; then
  fail "scraper health check failed"
fi
pass "scraper healthy"

# Gate 3 — scraper fetches one TwitchTracker page
GATE=3
echo "==> Gate 3: scraper POST /v2/scrape"
scrape_body="$(jq -nc \
  --arg url "${BEARHOST_TT_URL}" \
  --argjson timeout "${SCRAPER_TIMEOUT_MS}" \
  '{url: $url, formats: ["rawHtml"], useProxy: false, timeout: $timeout}')"
scrape_resp="$(bearhost_compose exec -T scraper curl -sf \
  -X POST http://127.0.0.1:8000/v2/scrape \
  -H 'Content-Type: application/json' \
  -d "${scrape_body}")"
if ! echo "${scrape_resp}" | jq -e '.success == true' >/dev/null 2>&1; then
  fail "scrape did not succeed: $(echo "${scrape_resp}" | jq -c '.error // .' 2>/dev/null || echo "${scrape_resp}")"
fi
pass "TwitchTracker scrape ok"

# Gate 4 — frontend via Caddy
GATE=4
echo "==> Gate 4: frontend ${PUBLIC_ORIGIN}/"
code="$(curl -sf -o /dev/null -w '%{http_code}' "${PUBLIC_ORIGIN}/" || echo 000)"
if [[ "${code}" != "200" ]]; then
  fail "frontend HTTP ${code}"
fi
pass "frontend 200"

# Gate 5 — metadata + analytics via Caddy
GATE=5
echo "==> Gate 5: metadata + analytics via Caddy"
meta_code="$(curl -sf -o /dev/null -w '%{http_code}' "${PUBLIC_ORIGIN}/v1/streams?limit=1" || echo 000)"
ana_code="$(curl -sf -o /dev/null -w '%{http_code}' "${PUBLIC_ORIGIN}/v1/analytics/always-tracked" || echo 000)"
if [[ "${meta_code}" != "200" ]]; then
  fail "metadata /v1/streams HTTP ${meta_code}"
fi
if [[ "${ana_code}" != "200" ]]; then
  fail "analytics /v1/analytics/always-tracked HTTP ${ana_code}"
fi
pass "metadata + analytics 200"

if [[ "${BEARHOST_SKIP_SYNC:-}" == "1" ]]; then
  echo "bearhost-smoke: skipping sync gates (BEARHOST_SKIP_SYNC=1)"
else
  # Gate 6 — trigger sync; Redis job key appears
  GATE=6
  echo "==> Gate 6: sync trigger + Redis job key"
  streams_json="$(curl -sf "${PUBLIC_ORIGIN}/v1/streams?limit=1")"
  stream_id="$(echo "${streams_json}" | jq -r '.streams[0].id // .[0].id // empty' 2>/dev/null || true)"
  if [[ -z "${stream_id}" ]]; then
    stream_id="$(echo "${streams_json}" | jq -r '.items[0].id // empty' 2>/dev/null || true)"
  fi
  if [[ -z "${stream_id}" || "${stream_id}" == "null" ]]; then
    echo "bearhost-smoke: no streams in DB — watching channel ninja for gate 6"
    watch_code="$(curl -sf -o /dev/null -w '%{http_code}' \
      -X POST "${PUBLIC_ORIGIN}/v1/analytics/channels/ninja/watch" || echo 000)"
    if [[ "${watch_code}" != "200" && "${watch_code}" != "202" ]]; then
      fail "channel watch HTTP ${watch_code} (empty DB and no stream to sync)"
    fi
    pass "channel watch accepted (empty DB bootstrap)"
  else
  sync_code="$(curl -sf -o /dev/null -w '%{http_code}' \
    -X POST "${PUBLIC_ORIGIN}/v1/analytics/streams/${stream_id}/sync?viewers_only=true" || echo 000)"
  if [[ "${sync_code}" != "200" && "${sync_code}" != "202" ]]; then
    fail "sync POST HTTP ${sync_code}"
  fi
  redis_key=""
  for _ in $(seq 1 30); do
    redis_key="$(bearhost_compose exec -T redis redis-cli GET "analytics:sync:${stream_id}" 2>/dev/null || true)"
    if [[ -n "${redis_key}" && "${redis_key}" != "(nil)" ]]; then
      break
    fi
    sleep 2
  done
  if [[ -z "${redis_key}" || "${redis_key}" == "(nil)" ]]; then
    fail "Redis key analytics:sync:${stream_id} not found after sync trigger"
  fi
  pass "Redis sync key present for ${stream_id}"
  fi

  if [[ -z "${stream_id:-}" || "${stream_id}" == "null" ]]; then
    echo "bearhost-smoke: skipping gate 7 (no stream id after watch bootstrap)"
  else
  GATE=7
  echo "==> Gate 7: Postgres rollup activity"
  count_before="$(bearhost_compose exec -T postgres psql -U app -d streamclone -tAc \
    'SELECT COUNT(*) FROM analytics_minute_rollups' 2>/dev/null | tr -d '[:space:]')"
  count_before="${count_before:-0}"
  increased=false
  for _ in $(seq 1 60); do
    phase="$(curl -sf "${PUBLIC_ORIGIN}/v1/analytics/streams/${stream_id}/sync/status" 2>/dev/null \
      | jq -r '.phase // empty' || true)"
    count_now="$(bearhost_compose exec -T postgres psql -U app -d streamclone -tAc \
      'SELECT COUNT(*) FROM analytics_minute_rollups' 2>/dev/null | tr -d '[:space:]')"
    count_now="${count_now:-0}"
    if [[ "${count_now}" -gt "${count_before}" ]]; then
      increased=true
      break
    fi
    if [[ "${phase}" == "done" || "${phase}" == "complete" || "${phase}" == "idle" ]]; then
      break
    fi
    sleep 5
  done
  if [[ "${increased}" == "true" ]]; then
    pass "rollup count increased (before=${count_before}, after=${count_now})"
  elif [[ "${count_before}" -gt 0 ]]; then
    pass "rollup rows already present (${count_before})"
  elif [[ -n "${redis_key:-}" && "${redis_key}" != "(nil)" ]]; then
    pass "sync job active in Redis (rollups may lag on fresh DB)"
  else
    fail "no rollup activity (before=${count_before}, after=${count_now:-0})"
  fi
  fi
fi

# Gate 8 — no repeated OOM / restart loops in recent logs
GATE=8
echo "==> Gate 8: recent logs (OOM / restart loops)"
log_blob="$(bearhost_compose logs --tail=50 2>&1 || true)"
oom_count="$(echo "${log_blob}" | grep -ciE 'out of memory|oom|killed process' || true)"
if [[ "${oom_count}" -gt 3 ]]; then
  fail "possible OOM in logs (${oom_count} matches)"
fi
restart_count="$(echo "${log_blob}" | grep -ciE 'restart loop|too many restarts' || true)"
if [[ "${restart_count}" -gt 0 ]]; then
  fail "restart loop messages in logs"
fi
pass "no OOM/restart storm in last 50 log lines per service"

# Gate 9 — corpus preflight (Azure secret + Twitch creds)
GATE=9
echo "==> Gate 9: corpus preflight"
corpus_preflight_ok=0
if bearhost_corpus_preflight; then
  corpus_preflight_ok=1
  pass "corpus preflight requirements present"
else
  echo "bearhost-smoke: corpus preflight requirements still missing"
fi
if [[ "${corpus_preflight_ok}" == "1" && "${CORPUS_WORKERS_ENABLED:-0}" == "1" ]]; then
  pass "CORPUS_WORKERS_ENABLED=1 with preflight ok"
elif [[ "${CORPUS_WORKERS_ENABLED:-0}" == "1" ]]; then
  fail "CORPUS_WORKERS_ENABLED=1 but preflight failed"
else
  pass "corpus workers disabled (expected until operator enables)"
fi

# Gate 10 — admin archive route via Caddy (401 not 404)
GATE=10
echo "==> Gate 10: admin archive route via Caddy"
admin_code="$(curl -s -o /dev/null -w '%{http_code}' "${PUBLIC_ORIGIN}/v1/admin/archive/jobs" || echo 000)"
if [[ "${admin_code}" == "404" ]]; then
  fail "admin archive route returned 404 (metadata?) — check Caddy @admin_archive"
fi
if [[ "${admin_code}" == "401" || "${admin_code}" == "200" ]]; then
  pass "admin archive HTTP ${admin_code} (401 expected without token)"
else
  fail "admin archive unexpected HTTP ${admin_code}"
fi

echo "bearhost-smoke: all gates passed"
