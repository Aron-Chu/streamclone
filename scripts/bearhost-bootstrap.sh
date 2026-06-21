#!/usr/bin/env bash
# Bootstrap BearHost VPS for Streamclone production (Ubuntu 24.04).
# Run as root on first login, then re-login as streamclone@.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Aron-Chu/streamclone/master/scripts/bearhost-bootstrap.sh | bash
# Or from a cloned repo:
#   sudo bash scripts/bearhost-bootstrap.sh
#
# Optional env:
#   STREAMCLONE_USER       — deploy user (default: streamclone)
#   STREAMCLONE_APP_DIR    — git checkout (default: /opt/streamclone/app)
#   SKIP_UFW=1             — skip firewall setup
#   SKIP_FAIL2BAN=1        — skip fail2ban install

set -euo pipefail

STREAMCLONE_USER="${STREAMCLONE_USER:-streamclone}"
STREAMCLONE_ROOT="${STREAMCLONE_ROOT:-/opt/streamclone}"
STREAMCLONE_APP_DIR="${STREAMCLONE_APP_DIR:-${STREAMCLONE_ROOT}/app}"
STREAMCLONE_BACKUPS_DIR="${STREAMCLONE_BACKUPS_DIR:-${STREAMCLONE_ROOT}/backups}"
STREAMCLONE_DATA_DIR="${STREAMCLONE_DATA_DIR:-${STREAMCLONE_ROOT}/data}"
SECRETS_DIR="${SECRETS_DIR:-/etc/streamclone/secrets}"

echo "==> Streamclone BearHost bootstrap"

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run as root (first SSH login): sudo bash $0" >&2
  exit 1
fi

echo "==> Installing base packages..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq \
  ca-certificates curl git jq htop ufw fail2ban

echo "==> Installing Docker Engine + compose plugin..."
if ! command -v docker >/dev/null 2>&1; then
  curl -fsSL https://get.docker.com | sh
fi
if ! docker compose version >/dev/null 2>&1; then
  apt-get install -y -qq docker-compose-plugin
fi

if ! id "${STREAMCLONE_USER}" >/dev/null 2>&1; then
  echo "==> Creating user ${STREAMCLONE_USER}..."
  useradd -m -s /bin/bash "${STREAMCLONE_USER}"
fi
usermod -aG docker "${STREAMCLONE_USER}"
usermod -aG sudo "${STREAMCLONE_USER}"

echo "==> Creating layout under ${STREAMCLONE_ROOT}..."
install -d -m 755 -o "${STREAMCLONE_USER}" -g "${STREAMCLONE_USER}" \
  "${STREAMCLONE_ROOT}" \
  "${STREAMCLONE_APP_DIR}" \
  "${STREAMCLONE_BACKUPS_DIR}" \
  "${STREAMCLONE_DATA_DIR}"

echo "==> Secrets directory ${SECRETS_DIR}..."
install -d -m 700 -o "${STREAMCLONE_USER}" -g "${STREAMCLONE_USER}" "${SECRETS_DIR}"

if [[ "${SKIP_UFW:-}" != "1" ]]; then
  echo "==> UFW: allow 22, 80, 443 only..."
  ufw --force reset >/dev/null
  ufw default deny incoming
  ufw default allow outgoing
  ufw allow OpenSSH
  ufw allow 80/tcp
  ufw allow 443/tcp
  ufw --force enable
fi

if [[ "${SKIP_FAIL2BAN:-}" != "1" ]]; then
  echo "==> fail2ban sshd jail..."
  systemctl enable fail2ban >/dev/null 2>&1 || true
  systemctl restart fail2ban >/dev/null 2>&1 || true
fi

echo
echo "==> Bootstrap complete."
echo "    Re-login as: ssh -i ~/.ssh/id_ed25519_bearhost_streamclone ${STREAMCLONE_USER}@141.11.243.103"
echo
echo "Next steps (as ${STREAMCLONE_USER}):"
echo "  git clone --depth 1 https://github.com/Aron-Chu/streamclone.git ${STREAMCLONE_APP_DIR}"
echo "  cd ${STREAMCLONE_APP_DIR} && cp .env.example .env"
echo "  # Copy Twitch OAuth, SCRAPER_API_KEY, optional proxy from local machine"
echo "  install -m 600 -o ${STREAMCLONE_USER} -g ${STREAMCLONE_USER} \\"
echo "    ~/azure-archive-connection-string ${SECRETS_DIR}/azure-archive-connection-string"
echo "  docker login ghcr.io   # PAT with read:packages if images are private"
echo "  docs/bearhost-production.md"
