#!/usr/bin/env bash
# BearHost corpus smoke — bronze run-once + jobs list (requires Azure secret + CORPUS_WORKERS_ENABLED=1).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

SECRET_FILE="${ARCHIVE_AZURE_CONNECTION_STRING_FILE:-${HOME}/.streamclone/azure-archive-connection-string}"
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

echo "==> bronze run-once (top 5 channels)"
go run ./cmd/backfill bronze run-once || exit 1

echo "==> jobs list"
go run ./cmd/backfill jobs list --limit=5

echo "==> coverage report"
go run ./cmd/backfill coverage report --since=7d

echo "bearhost-corpus-smoke: pass"
