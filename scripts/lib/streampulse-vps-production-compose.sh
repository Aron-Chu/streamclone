#!/usr/bin/env bash
# Shared compose stack for streampulse-vps production (API + DB + single corpus worker).
# shellcheck shell=bash

streampulse_vps_resolve_worker_key() {
  WORKER_KEY="${WORKER_KEY:-${HOME}/.ssh/id_ed25519}"
  if [[ ! -f "${WORKER_KEY}" && -f "/home/aron/.ssh/id_ed25519" ]]; then
    WORKER_KEY="/home/aron/.ssh/id_ed25519"
  fi
  if [[ ! -f "${WORKER_KEY}" ]]; then
    echo "streampulse-vps: SSH key not found (set WORKER_KEY)" >&2
    exit 1
  fi
}

streampulse_vps_production_env_local() {
  echo "deploy/env/profile-streampulse-vps-production.local.env"
}

# Build the same docker compose argv used for production deploy/restore/migrate.
streampulse_vps_production_compose_args() {
  local root="${1:?root required}"
  local env_local
  env_local="$(streampulse_vps_production_env_local)"
  if [[ ! -f "${root}/${env_local}" ]]; then
    echo "streampulse-vps: missing ${root}/${env_local}" >&2
    exit 1
  fi
  local -a args=(
    docker compose
    --project-name streamclone-production
    --env-file "${root}/.env"
    --env-file "${root}/deploy/env/profile-full.env"
    --env-file "${root}/${env_local}"
    -f "${root}/deploy/docker-compose.yml"
    -f "${root}/deploy/docker-compose.bearhost-build.yml"
    -f "${root}/deploy/docker-compose.bearhost-prod.yml"
    -f "${root}/deploy/docker-compose.bearhost-pulse.yml"
    -f "${root}/deploy/docker-compose.streampulse-vps-production.yml"
  )
  if [[ "${STREAMPULSE_USE_RELEASE_IMAGES:-0}" == "1" ]]; then
    if [[ -z "${IMAGE_TAG:-}" ]]; then
      echo "STREAMPULSE_USE_RELEASE_IMAGES=1 requires IMAGE_TAG (immutable GHCR tag)" >&2
      exit 1
    fi
    args+=(-f "${root}/deploy/docker-compose.release.yml")
  fi
  printf '%q ' "${args[@]}"
}

streampulse_vps_production_compose() {
  local root="${1:?root required}"
  shift
  # shellcheck disable=SC2046
  $(streampulse_vps_production_compose_args "${root}") "$@"
}

streampulse_vps_production_postgres_id() {
  local root="${1:?root required}"
  streampulse_vps_production_compose "${root}" ps -q postgres
}

streampulse_vps_production_service_running() {
  local root="$1"
  local service="$2"
  local cid
  cid="$(streampulse_vps_production_compose "${root}" ps -q "${service}" 2>/dev/null || true)"
  [[ -n "${cid}" ]]
}

streampulse_vps_corpus_worker_conflicts() {
  local root="${1:?root required}"
  local -a conflicts=()
  local line name status
  while IFS='|' read -r name status; do
    [[ -z "${name}" ]] && continue
    case "${name}" in
      streampulse-analytics-workers|streampulse-scraper)
        conflicts+=("${name} (${status})")
        ;;
    esac
  done < <(docker ps -a --format '{{.Names}}|{{.Status}}' 2>/dev/null || true)
  if [[ ${#conflicts[@]} -gt 0 ]]; then
    printf '%s\n' "${conflicts[@]}"
    return 0
  fi
  if docker compose -f "${root}/deploy/docker-compose.streampulse-vps-corpus.yml" ps -q 2>/dev/null | grep -q .; then
    docker compose -f "${root}/deploy/docker-compose.streampulse-vps-corpus.yml" ps --format '{{.Name}}|{{.Status}}'
    return 0
  fi
  return 1
}
