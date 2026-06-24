#!/usr/bin/env bash
# Run post-deploy canary on VPS with beta key from secrets (never printed).
set -euo pipefail
cd /opt/streamclone/app
if [[ -f /etc/streamclone/secrets/pulse-beta.env ]]; then
  set -a
  # shellcheck disable=SC1091
  source /etc/streamclone/secrets/pulse-beta.env
  set +a
  # Use first key if comma-separated
  export PULSE_BETA_KEY="${PULSE_BETA_KEYS%%,*}"
fi
export PULSE_SMOKE_BASE_URL=https://api.streampulse.stream
bash scripts/batch-q-post-canary.sh
