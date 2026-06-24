#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"
bearhost_ssh_config
echo "==> bearhost-pulse-api on ${BEARHOST_USER}@${BEARHOST_HOST}"
bearhost_ssh "cd '${BEARHOST_REMOTE_APP}' && bash scripts/bearhost-pulse-api.sh"
