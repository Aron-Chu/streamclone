#!/usr/bin/env bash
# Run bronze/VOD status ON the BearHost VPS (from your PC via SSH).
# Usage: bash scripts/bearhost-bronze-status-remote.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"

bearhost_ssh_config
echo "==> bearhost bronze status on ${BEARHOST_USER}@${BEARHOST_HOST}"
bearhost_ssh_script bearhost-bronze-status.sh
