#!/usr/bin/env bash
# Shared BearHost docker compose helpers. Source from ops scripts (do not execute directly).

bearhost_root_dir() {
  if [[ -n "${BEARHOST_ROOT:-}" ]]; then
    printf '%s\n' "${BEARHOST_ROOT}"
    return 0
  fi
  local here
  here="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
  printf '%s\n' "${here}"
}

bearhost_build_local() {
  if [[ -n "${BEARHOST_BUILD_LOCAL:-}" ]]; then
    [[ "${BEARHOST_BUILD_LOCAL}" == "1" ]]
    return
  fi
  local root
  root="$(bearhost_root_dir)"
  if [[ -f "${root}/deploy/env/profile-bearhost-prod.env" ]] \
    && grep -qE '^BEARHOST_BUILD_LOCAL=1' "${root}/deploy/env/profile-bearhost-prod.env"; then
    return 0
  fi
  return 1
}

bearhost_compose_files() {
  local root
  root="$(bearhost_root_dir)"
  local files=(
    -f "${root}/deploy/docker-compose.yml"
    -f "${root}/deploy/docker-compose.prod.yml"
    -f "${root}/deploy/docker-compose.bearhost-prod.yml"
  )
  if bearhost_build_local; then
    files+=(-f "${root}/deploy/docker-compose.bearhost-build.yml")
  else
    files+=(-f "${root}/deploy/docker-compose.release.yml")
  fi
  printf '%s\n' "${files[@]}"
}

bearhost_compose() {
  local root
  root="$(bearhost_root_dir)"
  local args=(
    docker compose
    --env-file "${root}/.env"
    --env-file "${root}/deploy/env/profile-full.env"
    --env-file "${root}/deploy/env/profile-archive.env"
    --env-file "${root}/deploy/env/profile-bearhost-prod.env"
    --env-file "${root}/deploy/env/profile-bearhost-corpus.env"
  )
  local f
  while IFS= read -r f; do
    args+=("${f}")
  done < <(bearhost_compose_files)
  args+=(--profile scraper "$@")
  "${args[@]}"
}

bearhost_compose_obs() {
  local root
  root="$(bearhost_root_dir)"
  local args=(
    docker compose
    --env-file "${root}/.env"
    --env-file "${root}/deploy/env/profile-full.env"
    --env-file "${root}/deploy/env/profile-archive.env"
    --env-file "${root}/deploy/env/profile-bearhost-prod.env"
    --env-file "${root}/deploy/env/profile-bearhost-corpus.env"
  )
  local f
  while IFS= read -r f; do
    args+=("${f}")
  done < <(bearhost_compose_files)
  args+=(
    -f "${root}/deploy/docker-compose.observability.yml"
    --profile scraper
    --profile observability
    "$@"
  )
  "${args[@]}"
}
