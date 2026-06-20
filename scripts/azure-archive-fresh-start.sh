#!/usr/bin/env bash
# One-shot Azure archive deploy: terraform apply + local credentials + smoke blob.
# Prerequisites: az login
#
# Usage:
#   bash scripts/azure-archive-fresh-start.sh
#
# Optional: copy terraform.tfvars.example → terraform.tfvars first to customize.

set -eu

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TF_DIR="${REPO_ROOT}/deploy/terraform/azure/archive"

if ! command -v az >/dev/null 2>&1 || ! command -v terraform >/dev/null 2>&1; then
  echo "Install tools first: bash scripts/install-azure-archive-tools.sh" >&2
  exit 1
fi

if ! az account show >/dev/null 2>&1; then
  echo "Run: az login" >&2
  exit 1
fi

echo "==> Subscription: $(az account show --query id -o tsv)"
echo "==> Account:      $(az account show --query user.name -o tsv)"
echo "==> Terraform:    ${TF_DIR}"

cd "${TF_DIR}"

if [[ ! -f terraform.tfvars ]]; then
  echo "==> No terraform.tfvars — using defaults from variables.tf"
  echo "    (Optional: cp terraform.tfvars.example terraform.tfvars)"
fi

terraform init -upgrade
terraform apply -input=false -auto-approve

echo
terraform output -raw setup_summary
echo
echo "==> Env snippet file: $(terraform output -raw env_local_snippet_file 2>/dev/null || echo '(write_local_credentials=false)')"
echo "==> Merge archive.env.local.snippet into your repo .env.local"
