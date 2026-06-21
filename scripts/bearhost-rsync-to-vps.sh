#!/usr/bin/env bash
# Rsync local Streamclone checkout (+ optional streamclone-scraper sibling) to BearHost VPS.
#
# Usage (WSL or Linux):
#   bash scripts/bearhost-rsync-to-vps.sh
#
# Overrides:
#   BEARHOST_HOST=141.11.243.103
#   BEARHOST_USER=streamclone
#   BEARHOST_SSH_KEY=~/.ssh/id_ed25519_bearhost_streamclone
#   BEARHOST_REMOTE_APP=/opt/streamclone/app
#   BEARHOST_REMOTE_SCRAPER=/opt/streamclone/streamclone-scraper

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOST="${BEARHOST_HOST:-141.11.243.103}"
USER="${BEARHOST_USER:-streamclone}"
SSH_KEY="${BEARHOST_SSH_KEY:-${HOME}/.ssh/id_ed25519_bearhost_streamclone}"
REMOTE_APP="${BEARHOST_REMOTE_APP:-/opt/streamclone/app}"
REMOTE_SCRAPER="${BEARHOST_REMOTE_SCRAPER:-/opt/streamclone/streamclone-scraper}"

if [[ ! -f "${SSH_KEY}" ]]; then
  echo "bearhost-rsync: SSH key not found: ${SSH_KEY}" >&2
  exit 1
fi

RSYNC_EXCLUDES=(
  --exclude .git
  --exclude node_modules
  --exclude frontend/node_modules
  --exclude .env
  --exclude .env.local
  --exclude runtime
  --exclude pg-data
  --exclude .codegraph
)

RSYNC_SSH=(ssh -i "${SSH_KEY}" -o StrictHostKeyChecking=accept-new)

echo "==> Sync app: ${ROOT}/ -> ${USER}@${HOST}:${REMOTE_APP}/"
rsync -avz --delete "${RSYNC_EXCLUDES[@]}" \
  -e "${RSYNC_SSH[*]}" \
  "${ROOT}/" "${USER}@${HOST}:${REMOTE_APP}/"

SCRAPER_LOCAL="${ROOT}/../streamclone-scraper"
if [[ -d "${SCRAPER_LOCAL}" ]]; then
  echo "==> Sync scraper: ${SCRAPER_LOCAL}/ -> ${USER}@${HOST}:${REMOTE_SCRAPER}/"
  rsync -avz --delete \
    --exclude .git \
    --exclude node_modules \
    --exclude __pycache__ \
    --exclude .venv \
    -e "${RSYNC_SSH[*]}" \
    "${SCRAPER_LOCAL}/" "${USER}@${HOST}:${REMOTE_SCRAPER}/"
else
  echo "WARN: ${SCRAPER_LOCAL} not found — rsync streamclone-scraper manually to ${REMOTE_SCRAPER}" >&2
fi

echo "bearhost-rsync: done"
