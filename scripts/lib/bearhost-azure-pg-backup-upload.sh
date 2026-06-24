#!/usr/bin/env bash
# Upload a gzip Postgres dump to Azure Blob (offsite nightly backup).
# Sourced by bearhost-pg-backup.sh and pg-restore-smoke-disposable.sh helpers.
# Never prints connection strings or secrets.

bearhost_azure_secret_file() {
  local default="${HOME}/.streamclone/azure-archive-connection-string"
  if [[ -n "${ARCHIVE_AZURE_CONNECTION_STRING_FILE:-}" && -f "${ARCHIVE_AZURE_CONNECTION_STRING_FILE}" ]]; then
    printf '%s\n' "${ARCHIVE_AZURE_CONNECTION_STRING_FILE}"
    return 0
  fi
  if [[ -f /run/streamclone-secrets/azure-archive-connection-string ]]; then
    printf '%s\n' /run/streamclone-secrets/azure-archive-connection-string
    return 0
  fi
  if [[ -f "${default}" ]]; then
    printf '%s\n' "${default}"
    return 0
  fi
  return 1
}

bearhost_pg_backup_blob_name() {
  local dump_path="$1"
  local base prefix date stamp
  base="$(basename "${dump_path}")"
  prefix="${ARCHIVE_AZURE_PREFIX:-streamclone}"
  prefix="${prefix#/}"
  prefix="${prefix%/}"
  date="$(date -u +%Y-%m-%d)"
  stamp="${base#streamclone-}"
  printf '%s/postgres/nightly/%s/streamclone-%s\n' "${prefix}" "${date}" "${stamp}"
}

bearhost_upload_pg_backup_to_azure() {
  local dump_path="$1"
  local secret_file container blob_name size_bytes
  if [[ ! -s "${dump_path}" ]]; then
    echo "bearhost-azure-pg-backup-upload: dump missing or empty: ${dump_path}" >&2
    return 1
  fi
  if ! command -v az >/dev/null 2>&1; then
    echo "bearhost-azure-pg-backup-upload: az CLI not installed" >&2
    return 1
  fi
  if ! secret_file="$(bearhost_azure_secret_file)"; then
    echo "bearhost-azure-pg-backup-upload: no Azure secret file configured" >&2
    return 1
  fi
  container="${ARCHIVE_AZURE_CONTAINER:-streamclone-archive}"
  blob_name="$(bearhost_pg_backup_blob_name "${dump_path}")"
  size_bytes="$(wc -c < "${dump_path}" | tr -d '[:space:]')"

  echo "bearhost-azure-pg-backup-upload: uploading to container=${container} blob=${blob_name} bytes=${size_bytes}"
  if ! az storage blob upload \
    --connection-string "$(tr -d '\r\n' < "${secret_file}")" \
    --container-name "${container}" \
    --name "${blob_name}" \
    --file "${dump_path}" \
    --overwrite \
    --only-show-errors >/dev/null; then
    echo "bearhost-azure-pg-backup-upload: upload failed for ${blob_name}" >&2
    return 1
  fi

  echo "bearhost-azure-pg-backup-upload: uploaded ${blob_name} (${size_bytes} bytes)"
  return 0
}
