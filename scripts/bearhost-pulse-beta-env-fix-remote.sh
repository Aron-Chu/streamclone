#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
source "${ROOT}/scripts/lib/bearhost-ssh.sh"
bearhost_ssh bash -s <<'REMOTE'
set -euo pipefail
cd /opt/streamclone/app
ENV_FILE=.env
KEY=$(grep -E '^PULSE_BETA_KEYS=' "$ENV_FILE" 2>/dev/null | tail -1 | cut -d= -f2- || true)
if [[ -z "${KEY}" ]]; then
  KEY=$(openssl rand -hex 24)
fi
# Strip broken lines and duplicate PULSE_BETA_KEYS entries
grep -v -E '^PULSE_BETA_KEYS=|^nCORPUS' "$ENV_FILE" > "${ENV_FILE}.tmp" || true
echo "PULSE_BETA_KEYS=${KEY}" >> "${ENV_FILE}.tmp"
mv "${ENV_FILE}.tmp" "$ENV_FILE"
echo "PULSE_BETA_KEYS set (length ${#KEY})"
bash scripts/bearhost-pulse-api.sh
REMOTE
echo ""
echo "Beta key for extension options (copy once):"
source "${ROOT}/scripts/lib/bearhost-ssh.sh"
bearhost_ssh "grep '^PULSE_BETA_KEYS=' /opt/streamclone/app/.env"
