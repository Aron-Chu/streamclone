#!/usr/bin/env bash
# Read-only counts for additional Azure blob prefixes (directory/, viewer_rollup/).
# Metadata only — no downloads or mutations.
#
# Usage: bash scripts/storage/azure-extra-prefixes.sh
# See docs/storage/README.md.
set -euo pipefail

CONN_FILE="${AZURE_CONN_FILE:-$HOME/.streamclone/azure-archive-connection-string}"
if [[ -f "/mnt/c/Users/Aron/.streamclone/azure-archive-connection-string" && ! -f "$CONN_FILE" ]]; then
  CONN_FILE="/mnt/c/Users/Aron/.streamclone/azure-archive-connection-string"
fi

if [[ ! -f "$CONN_FILE" ]]; then
  echo "ERROR: connection string file not found: $CONN_FILE" >&2
  exit 1
fi

export AZURE_STORAGE_CONNECTION_STRING
AZURE_STORAGE_CONNECTION_STRING="$(tr -d '\r\n' < "$CONN_FILE")"

CONTAINER="${AZURE_CONTAINER:-streamclone-archive}"
PREFIX="${AZURE_PREFIX:-streamclone/}"

for p in directory/ viewer_rollup/; do
  c=$(az storage blob list --container-name "$CONTAINER" --prefix "${PREFIX}${p}" --query "length(@)" -o tsv)
  b=$(az storage blob list --container-name "$CONTAINER" --prefix "${PREFIX}${p}" --query "[].properties.contentLength" -o tsv | awk '{s+=$1} END {print s+0}')
  echo "$p count=$c bytes=$b"
done
