#!/usr/bin/env bash
# Shared read-only SQL checks for BearHost analytics deploy gate.
# Sourced by bearhost-analytics-predeploy-gate.sh (local or SSH remote).
set -euo pipefail

bearhost_analytics_gate_checks() {
  section() {
    printf '\n=== %s ===\n' "$1"
  }

  psql_query() {
    docker exec streamclone-postgres-1 psql -U app -d streamclone -P pager=off -t -A -c "$1"
  }

  table_exists() {
    local table_name="$1"
    psql_query "
SELECT COUNT(*)
FROM information_schema.tables
WHERE table_schema='public' AND table_name='${table_name}';
" | tr -d '[:space:]'
  }

  section "UTC"
  date -u +"%Y-%m-%dT%H:%M:%SZ"

  section "analytics_minute_rollups source columns"
  psql_query "
SELECT column_name
FROM information_schema.columns
WHERE table_name='analytics_minute_rollups'
  AND column_name IN ('chat_source','source_confidence','chat_source_detail')
ORDER BY column_name;
" | sed '/^$/d' || true

  source_columns="$(psql_query "
SELECT count(*)
FROM information_schema.columns
WHERE table_name='analytics_minute_rollups'
  AND column_name IN ('chat_source','source_confidence','chat_source_detail');
" | tr -d '[:space:]')"

  section "analytics_streams chat metadata columns"
  psql_query "
SELECT column_name
FROM information_schema.columns
WHERE table_name='analytics_streams'
  AND column_name IN ('chat_state','chat_source','source_confidence','chat_coverage_pct')
ORDER BY column_name;
" | sed '/^$/d' || true

  stream_columns="$(psql_query "
SELECT count(*)
FROM information_schema.columns
WHERE table_name='analytics_streams'
  AND column_name IN ('chat_state','chat_source','source_confidence','chat_coverage_pct');
" | tr -d '[:space:]')"

  section "public emote materialization tables (WARN only)"
  public_emote_tables=0
  for table in public_emote_provider_hourly_rollups public_emote_materialization_runs; do
    if [[ "$(table_exists "${table}")" == "1" ]]; then
      public_emote_tables=$((public_emote_tables + 1))
      echo "present: ${table}"
    else
      echo "missing: ${table}"
    fi
  done

  section "IVR rollup leakage check (requires chat_source column)"
  if [[ "${source_columns}" == "3" ]]; then
    psql_query "
SELECT chat_source, source_confidence, COUNT(*)
FROM analytics_minute_rollups
WHERE chat_source='ivr'
GROUP BY 1,2;
" | sed '/^$/d' || true
  else
    echo "skipped: chat_source columns missing"
  fi

  migration_000050=FAIL
  block_recreate=1
  gate=FAIL
  reason="new binary reads migration 000050 columns"

  if [[ "${source_columns}" == "3" && "${stream_columns}" == "4" ]]; then
    migration_000050=PASS
    block_recreate=0
    gate=PASS
    reason="rollup and stream chat metadata columns present"
  elif [[ "${source_columns}" != "3" ]]; then
    reason="analytics_minute_rollups missing chat_source columns (have ${source_columns:-0}, need 3)"
  else
    reason="analytics_streams missing chat metadata columns (have ${stream_columns:-0}, need 4)"
  fi

  if [[ "${public_emote_tables}" == "2" ]]; then
    migration_public_emotes="PASS tables=${public_emote_tables}"
  elif [[ "${public_emote_tables}" == "0" ]]; then
    migration_public_emotes="WARN tables=${public_emote_tables}"
  else
    migration_public_emotes="WARN tables=${public_emote_tables}"
  fi

  if [[ "${migration_000050}" == "PASS" ]]; then
    ivr_shadow="allowed_after_code_deploy"
  else
    ivr_shadow="HOLD until migrate applies 000050_stream_chat_source.up.sql"
  fi

  section "Verdict"
  echo "MIGRATION_000050=${migration_000050} source_columns=${source_columns:-0}"
  echo "MIGRATION_PUBLIC_EMOTES=${migration_public_emotes}"
  echo "IVR_SHADOW_CANARY=${ivr_shadow}"
  echo "BLOCK_ANALYTICS_RECREATE=${block_recreate}"
  echo "ANALYTICS_DEPLOY_GATE=${gate}"
  echo "reason=${reason}"
  echo "new_binary_requires_000050=confirmed"

  if [[ "${gate}" == "FAIL" || "${block_recreate}" == "1" ]]; then
    return 1
  fi
  return 0
}

bearhost_analytics_gate_smoke_fixtures() {
  psql_query() {
    docker exec streamclone-postgres-1 psql -U app -d streamclone -P pager=off -t -A -c "$1"
  }

  local stream_line vod_line
  stream_line="$(psql_query "
SELECT s.stream_id || '|' || COUNT(r.*)
FROM analytics_streams s
JOIN analytics_minute_rollups r ON r.stream_id = COALESCE(s.canonical_stream_id, s.stream_id)
GROUP BY s.stream_id
ORDER BY COUNT(r.*) DESC
LIMIT 1;
" | tr -d '[:space:]')"

  vod_line="$(psql_query "
SELECT s.vod_id || '|' || s.stream_id || '|' || COUNT(r.*)
FROM analytics_streams s
JOIN analytics_minute_rollups r ON r.stream_id = COALESCE(s.canonical_stream_id, s.stream_id)
WHERE COALESCE(s.vod_id,'') <> ''
GROUP BY s.vod_id, s.stream_id
ORDER BY COUNT(r.*) DESC
LIMIT 1;
" | tr -d '[:space:]')"

  if [[ -n "${stream_line}" ]]; then
    echo "PULSE_SMOKE_STREAM_ID=${stream_line%%|*}"
    echo "PULSE_SMOKE_STREAM_ROLLUPS=${stream_line#*|}"
  fi
  if [[ -n "${vod_line}" ]]; then
    echo "PULSE_SMOKE_VOD_ID=${vod_line%%|*}"
    local rest="${vod_line#*|}"
    echo "PULSE_SMOKE_VOD_STREAM_ID=${rest%%|*}"
    echo "PULSE_SMOKE_VOD_ROLLUPS=${rest#*|}"
  fi
}
