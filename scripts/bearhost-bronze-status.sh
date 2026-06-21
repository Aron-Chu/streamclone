#!/usr/bin/env bash
# BearHost VPS ONLY: bronze VOD index + coverage for tier-0 roster (100–500 channels).
# Run on the VPS, or from your PC: bash scripts/bearhost-bronze-status-remote.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

# Always use Docker Go + VPS compose network — never local `go run` against dev machine.
export BEARHOST_USE_DOCKER_GO=1

run_go() {
  bash "${ROOT}/scripts/bearhost-go-run.sh" "$@"
}

echo "==> bronze VOD index progress"
run_go ./cmd/backfill bronze status

echo ""
echo "==> backfill queue summary"
run_go ./cmd/backfill status || true

echo ""
echo "==> coverage (last 7d)"
run_go ./cmd/backfill coverage report --since=7d

echo ""
echo "==> archive jobs (running/stale)"
run_go ./cmd/backfill jobs list --status=running --limit=5 || true
run_go ./cmd/backfill jobs list --status=stale --limit=5 || true

echo "bearhost-bronze-status: done"
