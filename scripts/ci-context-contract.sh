#!/usr/bin/env bash
# Owner-local Streamclone context gate — deterministic / fork-safe.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if ROOT="$(git -C "${SCRIPT_DIR}/.." rev-parse --show-toplevel 2>/dev/null)"; then
  :
else
  ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
fi
cd "${ROOT}"

fail() { echo "FAIL: $*" >&2; exit 1; }

grep -q 'streampulse-sdlc/AGENTS.md' AGENTS.md \
  || fail "AGENTS.md must link streampulse-sdlc/AGENTS.md"
[[ -f docs/streampulse-product-boundary.md ]] || fail "missing product boundary doc"
# Script is required by CI workflow when present; absence is tracked CI debt on
# master (adding the scanner body trips public-ops / detect-private-key hooks).
if [[ ! -f scripts/ci-public-topology-scan.sh ]]; then
  echo "WARN: missing scripts/ci-public-topology-scan.sh (CI debt; workflow soft-skips)"
fi

if grep -nE 'extension BFF|/v1/extension' .kiro/steering/laptopworker-hosting.md 2>/dev/null \
  | grep -viE 'not |watch-only|streampulse-backend|api.streampulse'; then
  fail "laptopworker-hosting.md still describes extension BFF on this stack"
fi

# Competing Pulse ownership claims in active plans/docs are blocked by product-boundary-strict in CI.
if [[ -f ../streampulse-sdlc/scripts/context-contract-check.py ]]; then
  PY=""
  for candidate in "${HOME}/.local/bin/python3.14" python3 python; do
    if command -v "${candidate}" >/dev/null 2>&1 && "${candidate}" -c "import sys" >/dev/null 2>&1; then
      PY="${candidate}"
      break
    fi
    if [[ -x "${candidate}" ]]; then
      PY="${candidate}"
      break
    fi
  done
  if [[ -n "${PY}" ]]; then
    "${PY}" ../streampulse-sdlc/scripts/context-contract-check.py \
      --mode repo --repo twitch-7tv-clone --repo-path "${ROOT}" --workspace-root "$(cd .. && pwd)"
  fi
fi

echo "streamclone owner-local context-contract OK"
