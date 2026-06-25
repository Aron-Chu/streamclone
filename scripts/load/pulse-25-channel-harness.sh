#!/usr/bin/env bash
# LOAD-001 — synthetic Pulse load harness (watch + poll).
#
# Modes:
#   dry-run     — validate target, roster, beta key presence, health; no watch mutations
#   smoke       — 2–5 channels (default 3), staggered watch + poll
#   staging-25  — 25 channels on isolated/staging/local target only
#
# Required:
#   PULSE_LOAD_TARGET          Base URL (e.g. http://127.0.0.1:8090)
#
# Hosted targets also require (never logged):
#   PULSE_LOAD_BETA_KEY        Single beta key, or
#   PULSE_LOAD_BETA_KEYS       Comma-separated keys for synthetic principals
#
# staging-25 guardrails:
#   PULSE_LOAD_STAGING_CONFIRM=1
#   Target must be localhost / 127.0.0.1 / *staging* / *.local — never api.streampulse.stream
#
# Optional:
#   PULSE_LOAD_CHANNEL_COUNT=3
#   PULSE_LOAD_STAGGER_MS=2000
#   PULSE_LOAD_PROMETHEUS_URL=http://127.0.0.1:9090
#   PULSE_LOAD_EVIDENCE_FILE=docs/pulse-extension/load-001-dry-run-evidence.txt
#   PULSE_LOAD_PRODUCTION_CAP=10
#
# Examples:
#   PULSE_LOAD_TARGET=https://api.streampulse.stream PULSE_LOAD_MODE=dry-run \
#     bash scripts/load/pulse-25-channel-harness.sh
#
#   PULSE_LOAD_TARGET=http://127.0.0.1:8090 PULSE_LOAD_MODE=smoke \
#     PULSE_LOAD_BETA_KEY=... PULSE_LOAD_PROMETHEUS_URL=http://127.0.0.1:9090 \
#     bash scripts/load/pulse-25-channel-harness.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "${ROOT}"

MODE="${PULSE_LOAD_MODE:-dry-run}"
export PULSE_LOAD_MODE="${MODE}"

if [[ -z "${PULSE_LOAD_TARGET:-}" ]]; then
  echo "FAIL: set PULSE_LOAD_TARGET (base URL)" >&2
  exit 2
fi

python3 "${ROOT}/scripts/load/pulse_harness.py" --mode "${MODE}" "$@"
