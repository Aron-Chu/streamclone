#!/usr/bin/env bash
# Local security checks aligned with CI (gitleaks + env validation).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "==> gitleaks"
if command -v gitleaks >/dev/null 2>&1; then
  gitleaks detect --source . --config .gitleaks.toml --verbose --redact
elif command -v pre-commit >/dev/null 2>&1; then
  pre-commit run gitleaks --all-files
else
  echo "Install gitleaks or pre-commit (make install-hooks)" >&2
  exit 1
fi

echo "==> validate-env"
bash scripts/validate-env.sh

echo "security-scan ok"
