#!/usr/bin/env bash
# Create ~/.streamclone operator secret directory with README manifest (no secret values).
set -euo pipefail

DIR="${STREAMCLONE_SECRETS_DIR:-${HOME}/.streamclone}"
mkdir -p "${DIR}"
chmod 700 "${DIR}" 2>/dev/null || true

README="${DIR}/README"
cat > "${README}" <<'EOF'
Streamclone operator secrets (local only — never commit)

Files in this directory:
  alertmanager-webhook-url     Discord webhook for Alertmanager (one line)
  azure-archive-connection-string   Azure Blob connection string
  archive.env.local.snippet    Optional snippet to merge into repo .env.local

Docs: streamclone docs/operator-secrets.md
Install BearHost webhook: bash scripts/bearhost-alertmanager-secret-install.sh
EOF

echo "operator-secrets-init: created ${DIR}"
echo "operator-secrets-init: see docs/operator-secrets.md"
ls -la "${DIR}"
