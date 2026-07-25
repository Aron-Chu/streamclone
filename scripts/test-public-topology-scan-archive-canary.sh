#!/usr/bin/env bash
# Canary: archive paths are covered by the public topology scanner (fail closed).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CANARY_DIR="docs/archive/agent-plans"
CANARY_FILE="${CANARY_DIR}/_topology-canary-DO-NOT-COMMIT.md"
mkdir -p "$CANARY_DIR"

cleanup() { rm -f "$CANARY_FILE"; }
trap cleanup EXIT

# Build a forbidden host token without embedding the scanner pattern in this file.
OCTET_A='23'
OCTET_B='173'
OCTET_C='152'
OCTET_D='156'
CANARY_HOST="${OCTET_A}.${OCTET_B}.${OCTET_C}.${OCTET_D}"
printf 'canary host %s must be detected in archives\n' "$CANARY_HOST" >"$CANARY_FILE"

if bash scripts/ci-public-topology-scan.sh; then
  echo "FAIL: scanner did not detect archive canary" >&2
  exit 1
fi

rm -f "$CANARY_FILE"
bash scripts/ci-public-topology-scan.sh

echo "test-public-topology-scan-archive-canary: OK"
