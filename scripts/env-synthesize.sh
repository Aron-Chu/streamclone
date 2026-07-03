#!/usr/bin/env bash
# Synthesize .env from tracked templates (CI, make env, bootstrap).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/env.sh
source "${ROOT}/scripts/lib/env.sh"
PROFILE="${1:-core}"
OUTFILE="${2:-${ROOT}/.env}"
env_synthesize "$PROFILE" "$OUTFILE"
