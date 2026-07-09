#!/usr/bin/env bash
# Pre-commit product boundary guard — strict on master when enabled, report-only elsewhere.
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "${ROOT}"

branch="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)"
strict="${STREAMCLONE_BOUNDARY_STRICT:-0}"

if [[ "${branch}" == "master" || "${branch}" == "main" ]]; then
  strict=1
fi

if [[ "${strict}" -eq 1 ]]; then
  exec bash scripts/check-product-boundary.sh --strict
fi

exec bash scripts/check-product-boundary.sh --preflight
