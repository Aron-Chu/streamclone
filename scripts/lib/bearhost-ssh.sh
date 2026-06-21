#!/usr/bin/env bash
# SSH helper for BearHost VPS ops (rsync, remote commands).
set -euo pipefail

bearhost_ssh_config() {
  BEARHOST_HOST="${BEARHOST_HOST:-141.11.243.103}"
  BEARHOST_USER="${BEARHOST_USER:-streamclone}"
  BEARHOST_SSH_KEY="${BEARHOST_SSH_KEY:-${HOME}/.ssh/id_ed25519_bearhost_streamclone}"
  BEARHOST_REMOTE_APP="${BEARHOST_REMOTE_APP:-/opt/streamclone/app}"
}

bearhost_ssh() {
  bearhost_ssh_config
  if [[ ! -f "${BEARHOST_SSH_KEY}" ]]; then
    echo "bearhost-ssh: SSH key not found: ${BEARHOST_SSH_KEY}" >&2
    exit 1
  fi
  ssh -i "${BEARHOST_SSH_KEY}" -o StrictHostKeyChecking=accept-new \
    "${BEARHOST_USER}@${BEARHOST_HOST}" "$@"
}

bearhost_ssh_script() {
  local script_name="$1"
  shift || true
  bearhost_ssh_config
  bearhost_ssh "cd '${BEARHOST_REMOTE_APP}' && BEARHOST_USE_DOCKER_GO=1 bash scripts/${script_name} $*"
}
