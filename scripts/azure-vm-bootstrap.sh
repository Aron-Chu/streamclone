#!/usr/bin/env bash
# Bootstrap Streamclone Azure archive VM: Docker, compose plugin, host Tailscale, GHCR hints.
# Run on the Ubuntu VM after Terraform apply + SSH (as a user with sudo).
#
# Usage (on VM):
#   curl -fsSL https://raw.githubusercontent.com/Aron-Chu/streamclone/master/scripts/azure-vm-bootstrap.sh | bash
# Or from a cloned repo:
#   bash scripts/azure-vm-bootstrap.sh
#
# Optional env:
#   STREAMCLONE_REPO_URL   — git clone URL (default: GitHub)
#   STREAMCLONE_INSTALL_DIR — checkout path (default: ~/streamclone-src)
#   TAILSCALE_HOSTNAME     — MagicDNS name (default: azure-streamclone)
#   SKIP_TAILSCALE=1       — skip tailscale install/up
#   SKIP_CLONE=1           — skip git clone (repo already present)

set -euo pipefail

STREAMCLONE_REPO_URL="${STREAMCLONE_REPO_URL:-https://github.com/Aron-Chu/streamclone.git}"
STREAMCLONE_INSTALL_DIR="${STREAMCLONE_INSTALL_DIR:-${HOME}/streamclone-src}"
TAILSCALE_HOSTNAME="${TAILSCALE_HOSTNAME:-azure-streamclone}"

echo "==> Streamclone Azure VM bootstrap"

if ! command -v sudo >/dev/null 2>&1; then
  echo "sudo is required" >&2
  exit 1
fi

echo "==> Installing base packages..."
sudo apt-get update -qq
sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
  ca-certificates curl git ufw jq

echo "==> Installing Docker Engine + compose plugin..."
if ! command -v docker >/dev/null 2>&1; then
  curl -fsSL https://get.docker.com | sudo sh
fi
sudo usermod -aG docker "${USER}" || true

if ! docker compose version >/dev/null 2>&1; then
  echo "docker compose plugin missing after get.docker.com — install Docker Desktop plugin or apt docker-compose-plugin" >&2
  exit 1
fi

echo "==> UFW: allow SSH; scraper :8000 on tailscale0 only (after Tailscale up)..."
sudo ufw --force reset >/dev/null
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow OpenSSH
# tailscale0 may not exist until tailscale up — rule is added idempotently below
sudo ufw --force enable

if [[ "${SKIP_TAILSCALE:-}" != "1" ]]; then
  echo "==> Installing host Tailscale (preferred for hybrid plane)..."
  if ! command -v tailscale >/dev/null 2>&1; then
    curl -fsSL https://tailscale.com/install.sh | sh
  fi
  echo "==> tailscale up — complete auth in browser if prompted..."
  sudo tailscale up --hostname="${TAILSCALE_HOSTNAME}" || true
  if ip link show tailscale0 >/dev/null 2>&1; then
    sudo ufw allow in on tailscale0 to any port 8000 proto tcp comment 'scraper tailnet only' || true
  fi
fi

mkdir -p "${HOME}/.streamclone"

if [[ "${SKIP_CLONE:-}" != "1" ]] && [[ ! -d "${STREAMCLONE_INSTALL_DIR}/.git" ]]; then
  echo "==> Cloning Streamclone source to ${STREAMCLONE_INSTALL_DIR}..."
  git clone --depth 1 "${STREAMCLONE_REPO_URL}" "${STREAMCLONE_INSTALL_DIR}"
fi

echo
echo "==> GHCR image pull (optional — set IMAGE_TAG in .env on VM):"
echo "    docker login ghcr.io   # PAT with read:packages"
echo "    cd ${STREAMCLONE_INSTALL_DIR} && cp .env.example .env"
echo "    # merge deploy/env/profile-azure-scraper.env (Mode A) or profile-azure-workers.env + profile-archive.env (Mode B)"
echo "    # mount ~/.streamclone/azure-archive-connection-string for Mode B archive export"
echo
echo "Mode A (remote scraper smoke):"
echo "  cd ${STREAMCLONE_INSTALL_DIR}"
echo "  docker compose --env-file .env \\"
echo "    --env-file deploy/env/profile-azure-scraper.env \\"
echo "    -f deploy/docker-compose.azure-scraper.yml \\"
echo "    -f deploy/docker-compose.release.yml up -d"
echo
echo "Mode B (full archive plane):"
echo "  docker compose --env-file .env \\"
echo "    --env-file deploy/env/profile-archive.env \\"
echo "    --env-file deploy/env/profile-azure-workers.env \\"
echo "    -f deploy/docker-compose.azure-archive-plane.yml \\"
echo "    -f deploy/docker-compose.release.yml up -d"
echo
echo "Local hybrid (dev PC): merge deploy/env/profile-local-hybrid.env — workers OFF, SCRAPER_API_URL=http://${TAILSCALE_HOSTNAME}:8000/v2/scrape"
echo "Docs: docs/azure-archive-plane.md"
echo
echo "Bootstrap complete. Re-login or 'newgrp docker' if docker group was added."
