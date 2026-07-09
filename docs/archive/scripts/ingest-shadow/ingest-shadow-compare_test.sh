#!/usr/bin/env bash
# Fixture test for ingest-shadow-compare.sh closed-only gate mode.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FIXTURE_DIR="$(mktemp -d)"
trap 'rm -rf "${FIXTURE_DIR}"' EXIT

cat > "${FIXTURE_DIR}/latest.jsonl" <<'JSONL'
{"key":{"streamID":"1","channel":"xqc","minute":"2026-07-08T12:00:00Z","closed":true},"legacyChat":100,"shadowChat":100,"legacyViewers":5,"shadowViewers":5,"match":true,"recordedAt":"2026-07-08T12:01:00Z"}
{"key":{"streamID":"1","channel":"xqc","minute":"2026-07-08T12:01:00Z","closed":false},"legacyChat":50,"shadowChat":0,"legacyViewers":5,"shadowViewers":0,"match":false,"reason":"legacy_zero_shadow_nonzero","recordedAt":"2026-07-08T12:01:30Z"}
JSONL

export INGEST_SHADOW_ARTIFACT_DIR="${FIXTURE_DIR}"

echo "==> full compare should FAIL (50% match)"
if INGEST_SHADOW_ARTIFACT_DIR="${FIXTURE_DIR}" bash "${SCRIPT_DIR}/ingest-shadow-compare.sh" 2 1; then
  echo "FAIL: expected full compare to fail"
  exit 1
fi

echo "==> closed-only compare should PASS (100% closed match)"
if ! INGEST_SHADOW_ARTIFACT_DIR="${FIXTURE_DIR}" bash "${SCRIPT_DIR}/ingest-shadow-compare.sh" --closed-only 2 1; then
  echo "FAIL: expected closed-only compare to pass"
  exit 1
fi

echo "PASS: ingest-shadow-compare closed-only fixture"
