#!/usr/bin/env bash
# BearHost corpus smoke — bronze run-once + jobs list (requires Azure secret + CORPUS_WORKERS_ENABLED=1).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# shellcheck source=scripts/bearhost-corpus-preflight.sh
source "${ROOT}/scripts/bearhost-corpus-preflight.sh"

DEFAULT_SECRET_FILE="$(bearhost_host_azure_secret_path)"
SECRET_FILE="${ARCHIVE_AZURE_CONNECTION_STRING_FILE:-${DEFAULT_SECRET_FILE}}"
if [[ ! -f "${SECRET_FILE}" && -f "${DEFAULT_SECRET_FILE}" ]]; then
  SECRET_FILE="${DEFAULT_SECRET_FILE}"
fi
if [[ ! -f "${SECRET_FILE}" ]]; then
  echo "bearhost-corpus-smoke BLOCKED: no Azure secret at ${SECRET_FILE}"
  exit 2
fi

if [[ "${CORPUS_WORKERS_ENABLED:-0}" != "1" ]]; then
  echo "bearhost-corpus-smoke BLOCKED: set CORPUS_WORKERS_ENABLED=1 after preflight"
  exit 2
fi

export ARCHIVE_AZURE_CONNECTION_STRING_FILE="${SECRET_FILE}"
export ARCHIVE_ENABLED=true
export BRONZE_ENABLED=true

run_go() {
  if [[ "${BEARHOST_USE_DOCKER_GO:-0}" == "1" ]] || ! command -v go >/dev/null 2>&1; then
    bash "${ROOT}/scripts/bearhost-go-run.sh" "$@"
  else
    go run "$@"
  fi
}

echo "==> bronze run-once (top 5 channels)"
run_go ./cmd/backfill bronze run-once || exit 1

echo "==> jobs list"
run_go ./cmd/backfill jobs list --limit=5

echo "==> coverage report"
run_go ./cmd/backfill coverage report --since=7d

echo "bearhost-corpus-smoke: pass"
