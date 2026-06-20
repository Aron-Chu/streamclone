#!/usr/bin/env bash
# Install Azure CLI + Terraform for Streamclone archive (WSL / Linux / macOS).
# Usage: bash scripts/install-azure-archive-tools.sh
# Docs: docs/azure-archive-setup.md

set -eu

echo "==> Streamclone Azure archive tooling"

if command -v az >/dev/null 2>&1; then
  echo "Azure CLI: $(az version --query '\"azure-cli\"' -o tsv 2>/dev/null || az version | head -1)"
else
  echo "==> Installing Azure CLI (Microsoft apt repo)..."
  if ! command -v curl >/dev/null 2>&1; then
    echo "Install curl first." >&2
    exit 1
  fi
  curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash
fi

if command -v terraform >/dev/null 2>&1; then
  echo "Terraform: $(terraform version | head -1)"
else
  echo "==> Installing Terraform (HashiCorp apt repo)..."
  sudo apt-get update -qq
  sudo apt-get install -y gnupg software-properties-common
  wget -O- https://apt.releases.hashicorp.com/gpg | sudo gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
  echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/hashicorp.list >/dev/null
  sudo apt-get update -qq
  sudo apt-get install -y terraform
fi

echo
echo "Next:"
echo "  az login"
echo "  bash scripts/azure-archive-fresh-start.sh"
