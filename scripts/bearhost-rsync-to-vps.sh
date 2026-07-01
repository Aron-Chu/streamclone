#!/usr/bin/env bash
# Rsync local Streamclone checkout (+ optional streamclone-scraper sibling) to BearHost VPS.
#
# Usage (WSL or Linux):
#   bash scripts/bearhost-rsync-to-vps.sh
#   bash scripts/bearhost-rsync-to-vps.sh --dry-run
#
# Overrides:
#   BEARHOST_HOST=141.11.243.103
#   BEARHOST_USER=streamclone
#   BEARHOST_SSH_KEY=~/.ssh/id_ed25519_bearhost_streamclone
#   BEARHOST_REMOTE_APP=/opt/streamclone/app
#   BEARHOST_REMOTE_SCRAPER=/opt/streamclone/streamclone-scraper
#   ALLOW_DIRTY=1 ALLOW_DIRTY_REASON="..."  # break-glass only

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/deploy-rsync.sh
source "${ROOT}/scripts/lib/deploy-rsync.sh"

HOST="${BEARHOST_HOST:-141.11.243.103}"
USER="${BEARHOST_USER:-streamclone}"
SSH_KEY="${BEARHOST_SSH_KEY:-${HOME}/.ssh/id_ed25519_bearhost_streamclone}"
REMOTE_APP="${BEARHOST_REMOTE_APP:-/opt/streamclone/app}"
REMOTE_SCRAPER="${BEARHOST_REMOTE_SCRAPER:-/opt/streamclone/streamclone-scraper}"

RSYNC_FLAGS=(-avz --delete)
DRY_RUN=0
if [[ "${1:-}" == "--dry-run" ]]; then
  RSYNC_FLAGS=(-avzn --delete)
  DRY_RUN=1
  shift
fi

if [[ ! -f "${SSH_KEY}" ]]; then
  echo "bearhost-rsync: SSH key not found: ${SSH_KEY}" >&2
  exit 1
fi

require_clean_deploy_tree "${ROOT}"
deploy_rsync_excludes

RSYNC_SSH=(ssh -i "${SSH_KEY}" -o StrictHostKeyChecking=accept-new)

echo "==> Sync app: ${ROOT}/ -> ${USER}@${HOST}:${REMOTE_APP}/"
rsync "${RSYNC_FLAGS[@]}" "${RSYNC_EXCLUDES[@]}" \
  -e "${RSYNC_SSH[*]}" \
  "${ROOT}/" "${USER}@${HOST}:${REMOTE_APP}/"

SCRAPER_LOCAL="${ROOT}/../streamclone-scraper"
if [[ -d "${SCRAPER_LOCAL}" ]]; then
  echo "==> Sync scraper: ${SCRAPER_LOCAL}/ -> ${USER}@${HOST}:${REMOTE_SCRAPER}/"
  rsync "${RSYNC_FLAGS[@]}" --delete \
    --exclude .git \
    --exclude node_modules \
    --exclude __pycache__ \
    --exclude .venv \
    -e "${RSYNC_SSH[*]}" \
    "${SCRAPER_LOCAL}/" "${USER}@${HOST}:${REMOTE_SCRAPER}/"
else
  echo "WARN: ${SCRAPER_LOCAL} not found — rsync streamclone-scraper manually to ${REMOTE_SCRAPER}" >&2
fi

if [[ "${DRY_RUN}" -eq 0 ]]; then
  record_deployed_sha "${ROOT}" "${REMOTE_APP}" "${USER}@${HOST}" "${SSH_KEY}"
fi

echo "bearhost-rsync: done"
