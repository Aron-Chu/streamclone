#!/usr/bin/env bash
# Ludwig IVR shadow → GQL → reconcile proof (local stack or WSL + Docker).
# Verdict helpers: shadow artifact, reconcile artifact, gql/canonical DB rows, no IVR writes.
#
# Usage:
#   bash scripts/bench/ivr-shadow-reconcile-proof.sh
#   CHANNEL=ludwig VOD=2804592918 bash scripts/bench/ivr-shadow-reconcile-proof.sh
#
# Requires: docker compose stack up (postgres reachable), .env with Twitch creds optional for GQL.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "${ROOT}"

CHANNEL="${CHANNEL:-ludwig}"
VOD="${VOD:-2804592918}"
STREAM_ID="${STREAM_ID:-bench-ivr-${CHANNEL}-${VOD}}"
COMPOSE="${COMPOSE:-docker compose -f deploy/docker-compose.yml}"
ART_DIR="${ART_DIR:-runtime/ivr-shadow}"
LOG="${LOG:-runtime/ivr-shadow-reconcile-proof.log}"
RESET_DB="${RESET_DB:-1}"

mkdir -p "$ART_DIR"
rm -f "$ART_DIR"/bench-ivr-"${CHANNEL}"-"${VOD}"*.json

echo "=== Migration 000050 chat_source columns ===" | tee "$LOG"
$COMPOSE exec -T postgres psql -U app -d streamclone -v ON_ERROR_STOP=1 -c \
  "SELECT column_name FROM information_schema.columns
   WHERE table_name='analytics_minute_rollups'
     AND column_name IN ('chat_source','source_confidence','chat_source_detail')
   ORDER BY 1;" | tee -a "$LOG"
COL_COUNT=$($COMPOSE exec -T postgres psql -U app -d streamclone -tAc \
  "SELECT count(*) FROM information_schema.columns
   WHERE table_name='analytics_minute_rollups'
     AND column_name IN ('chat_source','source_confidence','chat_source_detail');")
if [ "${COL_COUNT// /}" != "3" ]; then
  echo "FAIL: migration 000050 columns missing (got ${COL_COUNT})" | tee -a "$LOG"
  exit 2
fi

if [ "$RESET_DB" = "1" ]; then
  echo "=== Reset fixture stream rollups for clean proof ===" | tee -a "$LOG"
  $COMPOSE exec -T postgres psql -U app -d streamclone -v ON_ERROR_STOP=1 <<SQL | tee -a "$LOG"
DELETE FROM analytics_minute_rollups WHERE stream_id='${STREAM_ID}';
UPDATE analytics_streams SET
  chat_state='none', chat_source='none', source_confidence='none',
  chat_source_detail='', chat_coverage_pct=0, ivr_coverage_pct=0,
  live_coverage_pct=0, gql_coverage_pct=0,
  missing_windows_json='[]'::jsonb, source_windows_json='[]'::jsonb,
  last_source_upgrade_at=NULL
WHERE stream_id='${STREAM_ID}';
SQL
fi

echo "=== Twitch credential presence (names only) ===" | tee -a "$LOG"
for k in TWITCH_CLIENT_ID TWITCH_CLIENT_SECRET TWITCH_OAUTH_CLIENT_ID TWITCH_OAUTH_CLIENT_SECRET TWITCH_GQL_URL; do
  if grep -q "^${k}=" .env 2>/dev/null; then
    val=$(grep "^${k}=" .env | head -1 | cut -d= -f2-)
    if [ -n "$val" ] && [ "$val" != "changeme" ] && [ "$val" != "" ]; then
      echo "  $k=set" | tee -a "$LOG"
    else
      echo "  $k=empty" | tee -a "$LOG"
    fi
  else
    echo "  $k=missing" | tee -a "$LOG"
  fi
done

echo | tee -a "$LOG"
echo "=== DB rollups BEFORE ===" | tee -a "$LOG"
$COMPOSE exec -T postgres psql -U app -d streamclone -c \
  "SELECT chat_source, source_confidence, COALESCE(chat_source_detail,'') AS detail, count(*)
   FROM analytics_minute_rollups WHERE stream_id='${STREAM_ID}'
   GROUP BY 1,2,3 ORDER BY 4 DESC;" | tee -a "$LOG"

echo | tee -a "$LOG"
echo "=== Running shadow-reconcile fixture ===" | tee -a "$LOG"
START_MS=$(date +%s%3N 2>/dev/null || python3 -c 'import time; print(int(time.time()*1000))')
set +e
docker run --rm --network host \
  -v "${ROOT}:/src" -w /src \
  --env-file .env \
  -e DATABASE_URL='postgres://app:app@127.0.0.1:5432/streamclone?sslmode=disable' \
  -e REDIS_URL='redis://127.0.0.1:6379/0' \
  -e GOLD_IVR_ENABLED=true \
  -e GOLD_IVR_SHADOW_MODE=true \
  -e GOLD_IVR_LITE_ENABLED=false \
  -e GOLD_IVR_PEAKS_ONLY_ENABLED=false \
  -e GOLD_IVR_CANONICAL_REPLACE=false \
  -e GOLD_IVR_ENABLED_CHANNEL_ALLOWLIST="${CHANNEL}" \
  -e GOLD_IVR_SHADOW_ARTIFACT_DIR=/src/runtime/ivr-shadow \
  golang:1.25-alpine go run ./cmd/backfill gold shadow-reconcile fixture \
    --channel="${CHANNEL}" --vod="${VOD}" \
  2>&1 | tee -a "$LOG"
RUN_EXIT=${PIPESTATUS[0]}
set -e
END_MS=$(date +%s%3N 2>/dev/null || python3 -c 'import time; print(int(time.time()*1000))')
DURATION_MS=$((END_MS - START_MS))
echo "fixture_exit=$RUN_EXIT duration_ms=$DURATION_MS" | tee -a "$LOG"

echo | tee -a "$LOG"
echo "=== DB rollups AFTER ===" | tee -a "$LOG"
$COMPOSE exec -T postgres psql -U app -d streamclone -c \
  "SELECT chat_source, source_confidence, COALESCE(chat_source_detail,'') AS detail, count(*)
   FROM analytics_minute_rollups WHERE stream_id='${STREAM_ID}'
   GROUP BY 1,2,3 ORDER BY 4 DESC;" | tee -a "$LOG"

echo | tee -a "$LOG"
echo "=== IVR provisional row check ===" | tee -a "$LOG"
$COMPOSE exec -T postgres psql -U app -d streamclone -c \
  "SELECT count(*) AS ivr_rows FROM analytics_minute_rollups
   WHERE stream_id='${STREAM_ID}' AND chat_source='ivr';" | tee -a "$LOG"

echo | tee -a "$LOG"
echo "=== Artifacts ===" | tee -a "$LOG"
ls -lah "$ART_DIR"/bench-ivr-"${CHANNEL}"-"${VOD}"*.json 2>&1 | tee -a "$LOG" || true

echo | tee -a "$LOG"
echo "=== Artifact safety scan ===" | tee -a "$LOG"
for f in "$ART_DIR"/bench-ivr-"${CHANNEL}"-"${VOD}"*.json; do
  [ -f "$f" ] || continue
  echo "--- $f ---" | tee -a "$LOG"
  head -c 1200 "$f" | tee -a "$LOG"
  echo | tee -a "$LOG"
  if grep -Ei '"(text|message_id|raw_chat|username|user_id)"' "$f"; then
    echo "FAIL: sensitive key in $f" | tee -a "$LOG"
    exit 3
  fi
done

exit "$RUN_EXIT"
