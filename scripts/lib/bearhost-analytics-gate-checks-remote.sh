#!/usr/bin/env bash
# Remote entry for bearhost-analytics-predeploy-gate.sh (runs on VPS via SSH).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
# shellcheck source=scripts/lib/bearhost-analytics-gate-checks.sh
source "${ROOT}/scripts/lib/bearhost-analytics-gate-checks.sh"

bearhost_analytics_gate_checks
