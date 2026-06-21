#!/usr/bin/env bash
# Restore drill — rehydrate one stream from archive blobs without scrape (local Postgres + Azure).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

STREAM_ID="${STREAM_ID:-}"
if [[ -z "${STREAM_ID}" ]]; then
  echo "Usage: STREAM_ID=<id> bash scripts/archive-restore-drill.sh"
  exit 2
fi

SECRET_FILE="${ARCHIVE_AZURE_CONNECTION_STRING_FILE:-${HOME}/.streamclone/azure-archive-connection-string}"
if [[ ! -f "${SECRET_FILE}" ]]; then
  echo "archive-restore-drill BLOCKED: no Azure secret at ${SECRET_FILE}"
  exit 2
fi

export ARCHIVE_AZURE_CONNECTION_STRING_FILE="${SECRET_FILE}"
export ARCHIVE_ENABLED=true

echo "==> archive restore drill for stream ${STREAM_ID}"
go run ./cmd/archive restore --stream-id="${STREAM_ID}"

echo "archive-restore-drill: pass"
