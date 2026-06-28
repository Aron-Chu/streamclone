#!/usr/bin/env bash
# Remote smoke fixture export (stream/VOD with most rollups).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
# shellcheck source=scripts/lib/bearhost-analytics-gate-checks.sh
source "${ROOT}/scripts/lib/bearhost-analytics-gate-checks.sh"

bearhost_analytics_gate_smoke_fixtures
