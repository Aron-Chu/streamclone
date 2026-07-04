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

DEFAULT_SECRET_FILE="${HOME}/.streamclone/azure-archive-connection-string"
if [[ -f "/etc/streamclone/secrets/azure-archive-connection-string" ]]; then
  DEFAULT_SECRET_FILE="/etc/streamclone/secrets/azure-archive-connection-string"
fi
if [[ -f "/mnt/c/Users/Aron/.streamclone/azure-archive-connection-string" && ! -f "${DEFAULT_SECRET_FILE}" ]]; then
  DEFAULT_SECRET_FILE="/mnt/c/Users/Aron/.streamclone/azure-archive-connection-string"
fi

SECRET_FILE="${ARCHIVE_AZURE_CONNECTION_STRING_FILE:-${DEFAULT_SECRET_FILE}}"
if [[ ! -f "${SECRET_FILE}" ]]; then
  echo "archive-restore-drill BLOCKED: no Azure secret at ${SECRET_FILE}"
  echo "Set ARCHIVE_AZURE_CONNECTION_STRING_FILE or create ${DEFAULT_SECRET_FILE}" >&2
  exit 2
fi

export ARCHIVE_AZURE_CONNECTION_STRING_FILE="${SECRET_FILE}"
export ARCHIVE_ENABLED=true

run_go() {
  if ! command -v go >/dev/null 2>&1; then
    echo "archive-restore-drill BLOCKED: go not found on PATH" >&2
    exit 2
  fi
  go run "$@"
}

echo "==> archive restore drill for stream ${STREAM_ID}"
run_go ./cmd/archive restore --stream-id "${STREAM_ID}"

echo "archive-restore-drill: pass"
