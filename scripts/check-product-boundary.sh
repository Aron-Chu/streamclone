#!/usr/bin/env bash
# Streamclone public product boundary grep gate (Step 7 preflight + strict).
# See docs/streampulse-product-boundary.md and docs/split/mirror-verification.md.
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "${ROOT}"

MODE="${1:---preflight}"
STRICT="${STREAMCLONE_BOUNDARY_STRICT:-0}"
if [[ "${MODE}" == "--strict" ]]; then
  STRICT=1
fi

RG=(rg -n --no-heading
  'packages/pulse|test-analytics|profile-bearhost|profile-hosted|PULSE_|api\.streampulse\.stream|production-promotion|hosted-production|ingest-phase|/v1/extension|/v1/portal|pulse-live-coverage|ingest-core|storygraph|pulse-wire|make up-scraper|@streamclone/pulse'
  --glob '!docs/streampulse-product-boundary.md'
  --glob '!docs/split/**'
  --glob '!CHANGELOG.md'
  --glob '!.cursor/plans/**'
  --glob '!scripts/check-product-boundary.sh'
  --glob '!scripts/pre-commit-product-boundary-guard.sh'
  --glob '!scripts/pre-commit-public-ops-guard.sh'
  --glob '!docs/archive/**'
  --glob '!.kiro/specs/**'
  --glob '!.kiro/steering/pulse-wire.md'
)

if [[ "${STRICT}" -eq 1 ]]; then
  RG+=(
    --glob '!cmd/analytics/**'
    --glob '!cmd/backfill/**'
    --glob '!cmd/archive/**'
    --glob '!internal/analytics/**'
    --glob '!packages/pulse-core/**'
    --glob '!packages/pulse-charts/**'
    --glob '!packages/analytics-console/**'
  )
fi

mapfile -t HITS < <("${RG[@]}" . 2>/dev/null || true)

if [[ "${#HITS[@]}" -eq 0 ]]; then
  echo "check-product-boundary: OK (${MODE}, strict=${STRICT})"
  exit 0
fi

echo "check-product-boundary: ${#HITS[@]} hit(s) (${MODE}, strict=${STRICT})" >&2
if [[ "${#HITS[@]}" -gt 0 ]]; then
  printf '%s\n' "${HITS[@]}" | head -n 80 >&2 || true
  if [[ "${#HITS[@]}" -gt 80 ]]; then
    echo "... and $((${#HITS[@]} - 80)) more" >&2
  fi
fi

if [[ "${STRICT}" -eq 1 ]]; then
  echo "Strict Step 7 gate failed. Trim scripts/docs/install surfaces or delete legacy trees." >&2
  exit 1
fi

echo "Preflight mode: hits reported only (STREAMCLONE_BOUNDARY_STRICT=1 or --strict to fail)." >&2
exit 0
