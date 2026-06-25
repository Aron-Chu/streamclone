#!/usr/bin/env bash
# STOR-R2-004 — local/staging restore drill: R2 direct read, read-through hit, Azure fallback.
# Read-only against Azure + R2 staging. No archive_exports updates. No object mutations.
#
# Env (never commit):
#   R2_STAGING_ENV_FILE, AZURE_CONN_FILE
# Optional: ARCHIVE_DRILL_AZURE_FALLBACK_KEY (auto-probed when unset)
#
# See docs/storage/azure-to-r2-migration.md § Phase 4 / STOR-R2-004.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

CONN_FILE="${AZURE_CONN_FILE:-$HOME/.streamclone/azure-archive-connection-string}"
if [[ -f "/mnt/c/Users/Aron/.streamclone/azure-archive-connection-string" && ! -f "$CONN_FILE" ]]; then
  CONN_FILE="/mnt/c/Users/Aron/.streamclone/azure-archive-connection-string"
fi
ENV_FILE="${R2_STAGING_ENV_FILE:-$HOME/.streamclone/r2-staging-s3.env}"
if [[ -f "/mnt/c/Users/Aron/.streamclone/r2-staging-s3.env" && ! -f "$ENV_FILE" ]]; then
  ENV_FILE="/mnt/c/Users/Aron/.streamclone/r2-staging-s3.env"
fi
if [[ ! -f "$CONN_FILE" ]]; then
  echo "r2-restore-drill BLOCKED: missing Azure secret at ${CONN_FILE}"
  exit 2
fi
if [[ ! -f "$ENV_FILE" ]]; then
  echo "r2-restore-drill BLOCKED: missing R2 env at ${ENV_FILE}"
  exit 2
fi

set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a

ACCOUNT_ID="${CLOUDFLARE_ACCOUNT_ID:-51dd8007b22ac92482388d8b6cdbb6e3}"
R2_BUCKET="${ARCHIVE_R2_BUCKET:-streampulse-artifacts-staging}"
R2_ENDPOINT="${ARCHIVE_R2_ENDPOINT:-https://${ACCOUNT_ID}.r2.cloudflarestorage.com}"
KEY_DIR="${TMPDIR:-/tmp}/streamclone-r2-drill-$$"
mkdir -p "$KEY_DIR"
trap 'rm -rf "$KEY_DIR"' EXIT

if [[ -z "${AWS_ACCESS_KEY_ID:-}" || -z "${AWS_SECRET_ACCESS_KEY:-}" ]]; then
  echo "r2-restore-drill BLOCKED: R2 S3 keys missing in ${ENV_FILE}"
  exit 2
fi
printf '%s' "$AWS_ACCESS_KEY_ID" > "${KEY_DIR}/access-key-id"
printf '%s' "$AWS_SECRET_ACCESS_KEY" > "${KEY_DIR}/secret-access-key"

export ARCHIVE_PRIMARY_PROVIDER=azure
export ARCHIVE_READ_THROUGH=true
export ARCHIVE_DUAL_WRITE=false
export ARCHIVE_R2_LIVE_TEST=1
export ARCHIVE_R2_BUCKET="$R2_BUCKET"
export ARCHIVE_R2_ACCOUNT_ID="$ACCOUNT_ID"
export ARCHIVE_R2_PREFIX=archive
export ARCHIVE_R2_ENDPOINT="$R2_ENDPOINT"
export ARCHIVE_R2_ACCESS_KEY_ID_FILE="${KEY_DIR}/access-key-id"
export ARCHIVE_R2_SECRET_ACCESS_KEY_FILE="${KEY_DIR}/secret-access-key"
export ARCHIVE_AZURE_CONNECTION_STRING_FILE="$CONN_FILE"
export ARCHIVE_AZURE_STORAGE_ACCOUNT="${ARCHIVE_AZURE_STORAGE_ACCOUNT:-ststreamclone3lf6tt}"
export ARCHIVE_AZURE_CONTAINER="${ARCHIVE_AZURE_CONTAINER:-streamclone-archive}"
export ARCHIVE_AZURE_PREFIX="${ARCHIVE_AZURE_PREFIX:-streamclone}"

if [[ -z "${ARCHIVE_DRILL_AZURE_FALLBACK_KEY:-}" ]]; then
  export AZURE_STORAGE_CONNECTION_STRING
  AZURE_STORAGE_CONNECTION_STRING="$(tr -d '\r\n' < "$CONN_FILE")"
  FALLBACK_CANDIDATES=(
    "rollups/stream_id=317014684259/part-000.jsonl.gz"
    "rollups/stream_id=319181844960/part-000.jsonl.gz"
    "rollups/stream_id=318619637345/part-000.jsonl.gz"
  )
  for key in "${FALLBACK_CANDIDATES[@]}"; do
    azure_blob="${ARCHIVE_AZURE_PREFIX}/${key}"
    if az storage blob exists \
      --container-name "$ARCHIVE_AZURE_CONTAINER" \
      --name "$azure_blob" \
      --query exists -o tsv 2>/dev/null | grep -qx true; then
      if ! aws s3 ls "s3://${R2_BUCKET}/archive/${key}" --endpoint-url "$R2_ENDPOINT" >/dev/null 2>&1; then
        export ARCHIVE_DRILL_AZURE_FALLBACK_KEY="$key"
        echo "azure_fallback_key=${key}"
        break
      fi
    fi
  done
fi

if [[ -z "${ARCHIVE_DRILL_AZURE_FALLBACK_KEY:-}" ]]; then
  echo "WARN: could not auto-probe ARCHIVE_DRILL_AZURE_FALLBACK_KEY; azure fallback subtest will skip"
fi

echo "==> STOR-R2-004 restore drill (read-only)"
echo "    ARCHIVE_PRIMARY_PROVIDER=${ARCHIVE_PRIMARY_PROVIDER}"
echo "    ARCHIVE_READ_THROUGH=${ARCHIVE_READ_THROUGH}"
echo "    ARCHIVE_DUAL_WRITE=${ARCHIVE_DUAL_WRITE}"
echo "    ARCHIVE_R2_BUCKET=${ARCHIVE_R2_BUCKET}"

run_go_test() {
  if command -v go >/dev/null 2>&1; then
    go test "$@" -count=1 -v
  else
    docker run --rm \
      -v "${ROOT}:/src" -w /src \
      -e ARCHIVE_PRIMARY_PROVIDER -e ARCHIVE_READ_THROUGH -e ARCHIVE_DUAL_WRITE \
      -e ARCHIVE_R2_LIVE_TEST -e ARCHIVE_R2_BUCKET -e ARCHIVE_R2_ACCOUNT_ID \
      -e ARCHIVE_R2_PREFIX -e ARCHIVE_R2_ENDPOINT \
      -e ARCHIVE_R2_ACCESS_KEY_ID_FILE -e ARCHIVE_R2_SECRET_ACCESS_KEY_FILE \
      -e ARCHIVE_AZURE_CONNECTION_STRING_FILE -e ARCHIVE_AZURE_STORAGE_ACCOUNT \
      -e ARCHIVE_AZURE_CONTAINER -e ARCHIVE_AZURE_PREFIX \
      -e ARCHIVE_DRILL_AZURE_FALLBACK_KEY \
      -v "${KEY_DIR}:${KEY_DIR}:ro" \
      -v "${CONN_FILE}:${CONN_FILE}:ro" \
      golang:1.25-alpine go test "$@" -count=1 -v
  fi
}

run_go_test ./internal/archive/... -run TestR2RestoreDrillLive

echo "r2-restore-drill: pass"
