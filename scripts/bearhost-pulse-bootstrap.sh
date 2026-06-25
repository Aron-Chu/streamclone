#!/usr/bin/env bash
# One-time Pulse beta secret + switch BearHost to Pulse API mode.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"

bash "${ROOT}/scripts/bearhost-rsync-to-vps.sh"

bearhost_ssh 'set -e
if [[ ! -f /etc/streamclone/secrets/pulse-beta.env ]]; then
  sudo mkdir -p /etc/streamclone/secrets
  sudo sh -c "echo PULSE_BETA_KEYS=$(openssl rand -hex 24) > /etc/streamclone/secrets/pulse-beta.env"
  sudo chmod 600 /etc/streamclone/secrets/pulse-beta.env
  echo "==> created /etc/streamclone/secrets/pulse-beta.env (read key on VPS only)"
else
  echo "==> pulse-beta.env already exists"
fi
cd /opt/streamclone/app && bash scripts/bearhost-pulse-api.sh
echo ""
echo "Beta key (copy to extension options — do not commit):"
sudo cat /etc/streamclone/secrets/pulse-beta.env
'
