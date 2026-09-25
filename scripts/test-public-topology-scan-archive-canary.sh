#!/usr/bin/env bash
# Canary: archive paths are covered by the public topology scanner (fail closed),
# and a hit is reported by path and line only, never by its contents.
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

CANARY_DIR="docs/archive/agent-plans"
CANARY_FILE="${CANARY_DIR}/_topology-canary-DO-NOT-COMMIT.md"
OUTPUT="$(mktemp)"
mkdir -p "$CANARY_DIR"

cleanup() { rm -f "$CANARY_FILE" "$OUTPUT"; }
trap cleanup EXIT

# A synthetic SSH key name caught by the scanner's generic rules. It is
# assembled at run time so this file itself never matches.
PARTS=(id ed25519 canary)
CANARY_TOKEN="${PARTS[0]}_${PARTS[1]}_${PARTS[2]}"
printf 'IdentityFile ~/.ssh/%s\n' "$CANARY_TOKEN" >"$CANARY_FILE"

if bash scripts/ci-public-topology-scan.sh >"$OUTPUT" 2>&1; then
  echo "test-public-topology-scan-archive-canary: FAIL — scanner missed an archive-path key name" >&2
  exit 1
fi
if ! grep -q "topology-hit file=${CANARY_FILE} line=1" "$OUTPUT"; then
  echo "test-public-topology-scan-archive-canary: FAIL — hit not reported by path and line" >&2
  exit 1
fi
if grep -qF "$CANARY_TOKEN" "$OUTPUT"; then
  echo "test-public-topology-scan-archive-canary: FAIL — scanner printed the matched value" >&2
  exit 1
fi

rm -f "$CANARY_FILE"
bash scripts/ci-public-topology-scan.sh

echo "test-public-topology-scan-archive-canary: OK"
