#!/usr/bin/env bash
# Install Alertmanager webhook secret on BearHost without printing the URL.
# Usage (operator, local WSL):
#   1. Write one-line webhook URL to ~/.streamclone/alertmanager-webhook-url (mode 600)
#   2. bash scripts/bearhost-alertmanager-secret-install.sh
#   3. bash scripts/tmp/batch-b4f-observability-deploy.sh  # or bearhost-observability.sh up on VPS
set -euo pipefail

LOCAL_SECRET="${ALERTMANAGER_WEBHOOK_FILE:-${HOME}/.streamclone/alertmanager-webhook-url}"
KEY="${BEARHOST_SSH_KEY:-${HOME}/.ssh/id_ed25519_bearhost_streamclone}"
HOST="${BEARHOST_USER:-streamclone}@${BEARHOST_HOST:-141.11.243.103}"
REMOTE_DIR="/etc/streamclone/secrets"
REMOTE_FILE="${REMOTE_DIR}/alertmanager-webhook-url"

if [[ ! -s "${LOCAL_SECRET}" ]]; then
  echo "bearhost-alertmanager-secret-install: missing local secret at ${LOCAL_SECRET}" >&2
  echo "  Create file with one-line webhook URL (chmod 600). Do not commit." >&2
  exit 1
fi

echo "bearhost-alertmanager-secret-install: installing secret on BearHost (URL not printed)"

install_remote() {
  ssh -i "${KEY}" -o StrictHostKeyChecking=accept-new "${HOST}" \
    "cat > '${REMOTE_FILE}' && chmod 644 '${REMOTE_FILE}'" \
    < "${LOCAL_SECRET}"
}

if ssh -i "${KEY}" -o StrictHostKeyChecking=accept-new "${HOST}" \
  "test -d '${REMOTE_DIR}' && test -w '${REMOTE_DIR}'"; then
  install_remote
else
  ssh -i "${KEY}" -o StrictHostKeyChecking=accept-new "${HOST}" \
    "sudo install -d -m 750 -o streamclone -g streamclone '${REMOTE_DIR}'"
  ssh -i "${KEY}" -o StrictHostKeyChecking=accept-new "${HOST}" \
    "sudo tee '${REMOTE_FILE}' >/dev/null && sudo chown streamclone:streamclone '${REMOTE_FILE}' && sudo chmod 640 '${REMOTE_FILE}'" \
    < "${LOCAL_SECRET}"
fi

ssh -i "${KEY}" -o StrictHostKeyChecking=accept-new "${HOST}" \
  "test -s '${REMOTE_FILE}' && echo alertmanager-webhook-secret-present"

echo "bearhost-alertmanager-secret-install: done"
