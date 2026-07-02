#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

check_one_statement() {
  local file="${1:?file required}"
  local want="${2:?expected statement required}"
  local body
  body="$(tr -d '\r' < "${ROOT}/${file}" | sed '/^[[:space:]]*$/d')"
  if [[ "${body}" != *"${want}"* ]]; then
    echo "${file}: missing ${want}" >&2
    return 1
  fi
  local semicolons
  semicolons="$(printf '%s' "${body}" | grep -o ';' | wc -l | tr -d '[:space:]')"
  if [[ "${semicolons}" != "1" ]]; then
    echo "${file}: expected exactly one SQL statement, found ${semicolons}" >&2
    return 1
  fi
  if printf '%s' "${body}" | grep -qiE '(^|[[:space:]])(BEGIN|COMMIT)([[:space:]]|;|$)'; then
    echo "${file}: concurrent index migration must not include transaction statements" >&2
    return 1
  fi
  if printf '%s' "${body}" | grep -qiE 'CREATE[[:space:]]+INDEX' && ! printf '%s' "${body}" | grep -qiE 'CREATE[[:space:]]+INDEX[[:space:]]+CONCURRENTLY'; then
    echo "${file}: create index migration must use CONCURRENTLY" >&2
    return 1
  fi
  if printf '%s' "${body}" | grep -qiE 'DROP[[:space:]]+INDEX' && ! printf '%s' "${body}" | grep -qiE 'DROP[[:space:]]+INDEX[[:space:]]+CONCURRENTLY'; then
    echo "${file}: drop index migration must use CONCURRENTLY" >&2
    return 1
  fi
}

check_one_statement migrations/000059_analytics_rollups_minute_ts_index.up.sql 'CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_analytics_rollups_minute_ts'
check_one_statement migrations/000059_analytics_rollups_minute_ts_index.down.sql 'DROP INDEX CONCURRENTLY IF EXISTS idx_analytics_rollups_minute_ts'
check_one_statement migrations/000060_analytics_rollups_window_hot_index.up.sql 'CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_analytics_rollups_window_hot'
check_one_statement migrations/000060_analytics_rollups_window_hot_index.down.sql 'DROP INDEX CONCURRENTLY IF EXISTS idx_analytics_rollups_window_hot'

echo 'rollup index migrations OK'
