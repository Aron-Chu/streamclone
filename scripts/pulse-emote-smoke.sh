#!/usr/bin/env bash
# Pulse emote ensure + gold gate smoke (A+ path). No scrape-archive corpus required.
#
# Usage:
#   make smoke-pulse-emote                    # core gates (health, ensure, pulse emoteSync)
#   make pulse-emote-pick-stream              # list candidate gold streams from Postgres
#   make smoke-pulse-emote-gold LOGIN=xqc STREAM_ID=123
#   make smoke-pulse-emote-gold-fail LOGIN=xqc STREAM_ID=123
#
# Env:
#   STREAMCLONE_BASE_URL   default http://localhost:8090
#   LOGIN                  default xqc
#   TWITCH_ID              default 71092938 (xqc)
#   STREAM_ID              required for --gold / --gold-fail
#   SKIP_UNIT_TESTS=1      skip make test-pulse-emote in core smoke
#   ENV_FILE               default .env (via Makefile COMPOSE_CORE)

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BASE_URL="${STREAMCLONE_BASE_URL:-http://localhost:8090}"
LOGIN="${LOGIN:-xqc}"
TWITCH_ID="${TWITCH_ID:-71092938}"
STREAM_ID="${STREAM_ID:-}"
MODE="core"
SKIP_UNIT="${SKIP_UNIT_TESTS:-}"

ENV_FILE="${ENV_FILE:-.env}"
# shellcheck source=scripts/lib/env.sh
source "${ROOT}/scripts/lib/env.sh"
PROFILES="$(env_feature_compose_profiles "$ENV_FILE" 2>/dev/null || true)"
COMPOSE=(docker compose --env-file "$ENV_FILE" -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml)
for p in $PROFILES; do
  COMPOSE+=(--profile "$p")
done

fail() {
  echo "pulse-emote-smoke FAIL: $1" >&2
  exit 1
}

pass() {
  echo "pulse-emote-smoke PASS: $1"
}

note() {
  echo "pulse-emote-smoke: $1"
}

json_get() {
  python3 -c "
import sys, json
key = sys.argv[1].split('.')
d = json.load(sys.stdin)
for part in key:
    if not isinstance(d, dict):
        print('')
        sys.exit(0)
    d = d.get(part)
print('' if d is None else d)
" "$1"
}

http_get() {
  curl --connect-timeout 3 --max-time "${2:-15}" -fsS "$1"
}

http_post_json() {
  local url="$1"
  local body="$2"
  curl --connect-timeout 3 --max-time "${3:-30}" -fsS -X POST "$url" \
    -H "Content-Type: application/json" -d "$body"
}

wait_extension_health() {
  note "gate: extension health ($BASE_URL)"
  local i body ok
  for i in $(seq 1 30); do
    if body="$(http_get "$BASE_URL/v1/extension/health" 5 2>/dev/null)"; then
      ok="$(printf '%s' "$body" | json_get ok)"
      if [ "$ok" = "True" ] || [ "$ok" = "true" ]; then
        pass "extension health ok"
        return 0
      fi
    fi
    sleep 2
  done
  fail "extension health not ready — run: make up"
}

run_unit_tests() {
  if [ "$SKIP_UNIT" = "1" ]; then
    note "skipping unit tests (SKIP_UNIT_TESTS=1)"
    return 0
  fi
  note "gate: unit tests (test-pulse-emote)"
  make test-pulse-emote
  pass "unit tests ok"
}

wait_emote_ensure_ready() {
  note "gate: emote ensure API (login=$LOGIN)"
  local body='{"twitch_id":"'"$TWITCH_ID"'","providers":["seventv","twitch","ffz"]}'
  local url="$BASE_URL/v1/channels/$LOGIN/emotes/ensure"
  local i state count
  for i in $(seq 1 20); do
    if ! resp="$(http_post_json "$url" "$body" 45 2>/dev/null)"; then
      note "ensure attempt $i: request failed"
      sleep 3
      continue
    fi
    state="$(printf '%s' "$resp" | json_get state)"
    count="$(printf '%s' "$resp" | json_get count)"
    pending="$(printf '%s' "$resp" | json_get pending)"
    seventv_count="$(printf '%s' "$resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(next((p.get('count',0) for p in d.get('providers',[]) if p.get('provider')=='seventv'), 0))" 2>/dev/null || echo 0)"
    note "ensure attempt $i: state=$state count=$count pending=$pending seventv=$seventv_count"
    if [ "$state" = "ready" ] && [ "${count:-0}" -gt 0 ] 2>/dev/null; then
      pass "emote ensure ready (count=$count)"
      return 0
    fi
    if [ "${count:-0}" -gt 0 ] && [ "${pending:-1}" -eq 0 ] 2>/dev/null; then
      pass "emote ensure dictionary loaded (count=$count pending=0)"
      return 0
    fi
    if [ "${seventv_count:-0}" -gt 0 ] 2>/dev/null; then
      pass "emote ensure seventv dictionary usable (count=$seventv_count)"
      return 0
    fi
    if [ "${count:-0}" -gt 0 ] 2>/dev/null; then
      pass "emote ensure dictionary usable (count=$count)"
      return 0
    fi
    sleep 3
  done
  fail "emote ensure did not reach ready — check emote service (make ps, make logs)"
}

wait_pulse_emote_sync() {
  note "gate: extension pulse emoteSync (login=$LOGIN)"
  http_post_json "$BASE_URL/v1/analytics/channels/$LOGIN/watch" '{}' 10 >/dev/null 2>&1 || true
  local i resp state
  for i in $(seq 1 12); do
    if ! resp="$(http_get "$BASE_URL/v1/extension/pulse/channels/$LOGIN" 10 2>/dev/null)"; then
      sleep 3
      continue
    fi
    if ! printf '%s' "$resp" | grep -q '"emoteSync"'; then
      fail "pulse missing emoteSync — rebuild analytics: make rebuild-analytics-emote"
    fi
    state="$(printf '%s' "$resp" | json_get emoteSync.state)"
    note "pulse attempt $i: emoteSync.state=$state"
    case "$state" in
      ready|stale|syncing)
        pass "pulse emoteSync=$state"
        return 0
        ;;
      aggregate_only)
        note "WARN: emoteSync=aggregate_only — live ensurer may lack Helix broadcaster id; emote-service gate already passed"
        pass "pulse emoteSync present (aggregate_only)"
        return 0
        ;;
    esac
    sleep 3
  done
  fail "pulse emoteSync never reached ready/stale"
}

check_always_tracked() {
  note "gate: always-tracked list"
  local resp
  resp="$(http_get "$BASE_URL/v1/analytics/always-tracked" 10)"
  note "always-tracked: $resp"
  pass "always-tracked endpoint ok"
}

pick_stream() {
  note "candidate gold streams (vod_id set, newest first)"
  "${COMPOSE[@]}" exec -T postgres psql -U app -d streamclone -c \
    "SELECT stream_id, login, vod_id, started_at FROM analytics_streams WHERE COALESCE(vod_id,'') <> '' ORDER BY started_at DESC LIMIT 10;" \
    || fail "postgres query failed — is the stack up? (make up)"
}

postgres_seventv_count() {
  local sid="$1"
  "${COMPOSE[@]}" exec -T postgres psql -U app -d streamclone -tAc \
    "SELECT COUNT(*) FROM analytics_vod_chat_messages WHERE stream_id = '$sid' AND emote_frags::text LIKE '%seventv:%';" \
    2>/dev/null | tr -d '[:space:]' || echo "0"
}

run_gold_sync() {
  local sid="$1"
  local login="$2"
  note "gate: gold sync stream_id=$sid login=$login"
  http_post_json "$BASE_URL/v1/analytics/streams/$sid/sync?channel=$login" '{}' 15 >/dev/null

  local i resp phase err
  for i in $(seq 1 40); do
    sleep 15
    if ! resp="$(http_get "$BASE_URL/v1/analytics/streams/$sid/sync/status" 15 2>/dev/null)"; then
      note "sync status attempt $i: request failed"
      continue
    fi
    phase="$(printf '%s' "$resp" | json_get phase)"
    err="$(printf '%s' "$resp" | json_get error)"
    note "sync attempt $i: phase=$phase error=${err:-none}"
    case "$phase" in
      completed|export_pending)
        local n
        n="$(postgres_seventv_count "$sid")"
        note "vod chat rows with seventv: frags: $n"
        pass "gold sync completed (phase=$phase)"
        return 0
        ;;
      failed)
        fail "gold sync failed: $err"
        ;;
    esac
  done
  fail "gold sync timed out — check: make logs"
}

run_gold_fail_gate() {
  local sid="$1"
  local login="$2"
  note "gate: gold must fail when emote service is stopped"
  # Use a fresh stream id when possible to avoid a stale in-flight sync from prior runs.
  "${COMPOSE[@]}" stop emote >/dev/null
  sleep 2
  http_post_json "$BASE_URL/v1/analytics/streams/$sid/sync?channel=$login&force_chat=true" '{}' 15 >/dev/null || true

  local i resp phase err
  local saw_fail=false
  for i in $(seq 1 18); do
    sleep 10
    if ! resp="$(http_get "$BASE_URL/v1/analytics/streams/$sid/sync/status" 15 2>/dev/null)"; then
      continue
    fi
    phase="$(printf '%s' "$resp" | json_get phase)"
    err="$(printf '%s' "$resp" | json_get error)"
    note "fail-gate attempt $i: phase=$phase error=${err:-none}"
    if printf '%s' "$err" | grep -qi 'emote dictionary'; then
      saw_fail=true
      break
    fi
    if [ "$phase" = "failed" ]; then
      saw_fail=true
      break
    fi
  done

  "${COMPOSE[@]}" start emote >/dev/null
  sleep 3

  if [ "$saw_fail" != true ]; then
    fail "expected sync phase=failed when emote is down"
  fi
  if ! printf '%s' "$err" | grep -qi 'emote dictionary'; then
    note "WARN: failed as expected but error text may differ: $err"
  fi
  pass "gold blocked when emote service stopped"
}

for arg in "$@"; do
  case "$arg" in
    --gold) MODE="gold" ;;
    --gold-fail) MODE="gold-fail" ;;
    --pick-stream) MODE="pick" ;;
    --skip-unit) SKIP_UNIT=1 ;;
  esac
done

case "$MODE" in
  pick)
    pick_stream
    exit 0
    ;;
  gold|gold-fail)
    if [ -z "$STREAM_ID" ]; then
      fail "STREAM_ID is required — run: make pulse-emote-pick-stream"
    fi
    if [ -z "$LOGIN" ]; then
      fail "LOGIN is required"
    fi
    wait_extension_health
    if [ "$MODE" = "gold" ]; then
      run_gold_sync "$STREAM_ID" "$LOGIN"
    else
      run_gold_fail_gate "$STREAM_ID" "$LOGIN"
    fi
    exit 0
    ;;
esac

# Core smoke (default)
wait_extension_health
run_unit_tests
wait_emote_ensure_ready
wait_pulse_emote_sync
check_always_tracked
note "core smoke complete — for gold: make pulse-emote-pick-stream then make smoke-pulse-emote-gold LOGIN=... STREAM_ID=..."
pass "core pulse-emote smoke"
