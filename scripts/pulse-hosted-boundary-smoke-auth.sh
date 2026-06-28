#!/usr/bin/env bash
# Authenticated pulse-hosted-boundary-smoke (loads beta key from BearHost; never prints key).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"
raw="$(bearhost_ssh "grep -E '^PULSE_BETA_KEYS=' /etc/streamclone/secrets/pulse-beta.env | head -1")"
export PULSE_BETA_KEY="${raw#PULSE_BETA_KEYS=}"
PULSE_BETA_KEY="${PULSE_BETA_KEY%%,*}"
PULSE_BETA_KEY="${PULSE_BETA_KEY#\"}"
PULSE_BETA_KEY="${PULSE_BETA_KEY%\"}"
export PULSE_BETA_KEY
exec bash "${ROOT}/scripts/pulse-hosted-boundary-smoke.sh" "$@"
