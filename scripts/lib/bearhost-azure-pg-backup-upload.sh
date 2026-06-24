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
  if [[ -f /etc/streamclone/secrets/azure-archive-connection-string ]]; then
    printf '%s\n' /etc/streamclone/secrets/azure-archive-connection-string
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

bearhost_az_storage_blob_upload() {
  local secret_file="$1" container="$2" blob_name="$3" dump_path="$4"
  local conn dump_dir dump_base
  conn="$(tr -d '\r\n' < "${secret_file}")"
  if command -v az >/dev/null 2>&1; then
    az storage blob upload \
      --connection-string "${conn}" \
      --container-name "${container}" \
      --name "${blob_name}" \
      --file "${dump_path}" \
      --overwrite \
      --only-show-errors
    return $?
  fi
  if command -v docker >/dev/null 2>&1; then
    dump_dir="$(cd "$(dirname "${dump_path}")" && pwd)"
    dump_base="$(basename "${dump_path}")"
    AZURE_STORAGE_CONNECTION_STRING="${conn}" docker run --rm \
      -e AZURE_STORAGE_CONNECTION_STRING \
      -v "${dump_dir}:/dump:ro" \
      mcr.microsoft.com/azure-cli:2.67.0 \
      az storage blob upload \
      --container-name "${container}" \
      --name "${blob_name}" \
      --file "/dump/${dump_base}" \
      --overwrite \
      --only-show-errors
    return $?
  fi
  echo "bearhost-azure-pg-backup-upload: need az CLI or docker" >&2
  return 1
}

bearhost_upload_pg_backup_to_azure() {
  local dump_path="$1"
  local secret_file container blob_name size_bytes
  if [[ ! -s "${dump_path}" ]]; then
    echo "bearhost-azure-pg-backup-upload: dump missing or empty: ${dump_path}" >&2
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
  if ! bearhost_az_storage_blob_upload "${secret_file}" "${container}" "${blob_name}" "${dump_path}" >/dev/null; then
    echo "bearhost-azure-pg-backup-upload: upload failed for ${blob_name}" >&2
    return 1
  fi

  echo "bearhost-azure-pg-backup-upload: uploaded ${blob_name} (${size_bytes} bytes)"
  return 0
}
