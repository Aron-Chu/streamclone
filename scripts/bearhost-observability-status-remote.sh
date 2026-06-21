#!/usr/bin/env bash
# Observability stack status on BearHost VPS (from your PC via SSH).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"

bearhost_ssh_config
echo "==> bearhost observability status on ${BEARHOST_USER}@${BEARHOST_HOST}"
bearhost_ssh_script bearhost-observability.sh status
