#!/usr/bin/env bash
# Restore drill — rehydrate one stream from archive blobs without scrape (local Postgres + Azure).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# shellcheck source=scripts/bearhost-corpus-preflight.sh
source "${ROOT}/scripts/bearhost-corpus-preflight.sh"

STREAM_ID="${STREAM_ID:-}"
if [[ -z "${STREAM_ID}" ]]; then
  echo "Usage: STREAM_ID=<id> bash scripts/archive-restore-drill.sh"
  exit 2
fi

DEFAULT_SECRET_FILE="$(bearhost_host_azure_secret_path)"
SECRET_FILE="${ARCHIVE_AZURE_CONNECTION_STRING_FILE:-${DEFAULT_SECRET_FILE}}"
if [[ ! -f "${SECRET_FILE}" && -f "${DEFAULT_SECRET_FILE}" ]]; then
  SECRET_FILE="${DEFAULT_SECRET_FILE}"
fi
if [[ ! -f "${SECRET_FILE}" ]]; then
  echo "archive-restore-drill BLOCKED: no Azure secret at ${SECRET_FILE}"
  exit 2
fi

export ARCHIVE_AZURE_CONNECTION_STRING_FILE="${SECRET_FILE}"
export ARCHIVE_ENABLED=true

run_go() {
  if [[ "${BEARHOST_USE_DOCKER_GO:-0}" == "1" ]] || ! command -v go >/dev/null 2>&1; then
    bash "${ROOT}/scripts/bearhost-go-run.sh" "$@"
  else
    go run "$@"
  fi
}

echo "==> archive restore drill for stream ${STREAM_ID}"
run_go ./cmd/archive restore --stream-id="${STREAM_ID}"

echo "archive-restore-drill: pass"
