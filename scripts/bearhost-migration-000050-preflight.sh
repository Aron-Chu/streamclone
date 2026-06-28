#!/usr/bin/env bash
# Backward-compatible wrapper for migration 000050 preflight.
# Emits legacy MIGRATION_000050= and IVR_SHADOW_CANARY= lines via the full gate script.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec bash "${ROOT}/scripts/bearhost-analytics-predeploy-gate.sh" "$@"
