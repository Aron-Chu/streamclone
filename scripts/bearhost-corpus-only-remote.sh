#!/usr/bin/env bash
# Run bearhost-corpus-only.sh on the BearHost VPS from your PC (WSL).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"

bearhost_ssh_config
echo "==> bearhost-corpus-only on ${BEARHOST_USER}@${BEARHOST_HOST}"
bearhost_ssh "cd '${BEARHOST_REMOTE_APP}' && bash scripts/bearhost-corpus-only.sh"
