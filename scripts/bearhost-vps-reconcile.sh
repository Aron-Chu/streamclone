#!/usr/bin/env bash
set -euo pipefail
sed -i 's|^ARCHIVE_AZURE_CONNECTION_STRING_FILE=.*|ARCHIVE_AZURE_CONNECTION_STRING_FILE=/run/streamclone-secrets/azure-archive-connection-string|' /opt/streamclone/app/.env
cd /opt/streamclone/app
bash scripts/bearhost-deploy-phased.sh
