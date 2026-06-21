#!/usr/bin/env bash
# Run Streamclone Go CLIs inside a disposable Docker Go container on the compose network.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

# shellcheck source=scripts/bearhost-corpus-preflight.sh
source "${ROOT}/scripts/bearhost-corpus-preflight.sh"

if [[ $# -lt 2 ]]; then
  echo "Usage: scripts/bearhost-go-run.sh ./cmd/<tool> <args...>" >&2
  exit 2
fi

bearhost_build_local() {
  if [[ -n "${BEARHOST_BUILD_LOCAL:-}" ]]; then
    [[ "${BEARHOST_BUILD_LOCAL}" == "1" ]]
    return
  fi
  if [[ -f deploy/env/profile-bearhost-prod.env ]] \
    && grep -qE '^BEARHOST_BUILD_LOCAL=1' deploy/env/profile-bearhost-prod.env; then
    return 0
  fi
  return 1
}

bearhost_compose() {
  local compose_args=(
    docker compose
    --env-file .env
    --env-file deploy/env/profile-full.env
    --env-file deploy/env/profile-archive.env
    --env-file deploy/env/profile-bearhost-prod.env
    -f deploy/docker-compose.yml
    -f deploy/docker-compose.prod.yml
    -f deploy/docker-compose.bearhost-prod.yml
  )
  if bearhost_build_local; then
    compose_args+=(-f deploy/docker-compose.bearhost-build.yml)
  else
    compose_args+=(-f deploy/docker-compose.release.yml)
  fi
  compose_args+=(--profile scraper "$@")
  "${compose_args[@]}"
}

bearhost_go_network() {
  if [[ -n "${BEARHOST_DOCKER_NETWORK:-}" ]]; then
    printf '%s\n' "${BEARHOST_DOCKER_NETWORK}"
    return 0
  fi

  local container_ids=()
  local cid=""
  while IFS= read -r cid; do
    if [[ -n "${cid}" ]]; then
      container_ids+=("${cid}")
    fi
  done < <(bearhost_compose ps -q analytics-workers analytics postgres redis 2>/dev/null || true)

  local network=""
  for cid in "${container_ids[@]}"; do
    network="$(docker inspect -f '{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}' "${cid}" \
      | tr -d '\r' \
      | grep -E '_default$' \
      | sed -n '1p' || true)"
    if [[ -n "${network}" ]]; then
      printf '%s\n' "${network}"
      return 0
    fi

    network="$(docker inspect -f '{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}' "${cid}" \
      | tr -d '\r' \
      | sed -n '1p' || true)"
    if [[ -n "${network}" ]]; then
      printf '%s\n' "${network}"
      return 0
    fi
  done

  if docker network inspect streamclone_default >/dev/null 2>&1; then
    printf 'streamclone_default\n'
    return 0
  fi

  network="$(docker network ls --format '{{.Name}}' | grep -E '(^streamclone_default$|_default$)' | sed -n '1p' || true)"
  if [[ -n "${network}" ]]; then
    printf '%s\n' "${network}"
    return 0
  fi

  echo "bearhost-go-run BLOCKED: could not determine compose Docker network" >&2
  exit 2
}

network="$(bearhost_go_network)"
secret_dir="${STREAMCLONE_SECRETS_DIR:-/etc/streamclone/secrets}"
if [[ ! -d "${secret_dir}" ]]; then
  secret_dir="$(dirname "$(bearhost_host_azure_secret_path)")"
fi

if [[ ! -d "${secret_dir}" ]]; then
  echo "bearhost-go-run BLOCKED: secret directory not found: ${secret_dir}" >&2
  exit 2
fi

env_files=(
  ".env"
  "deploy/env/profile-full.env"
  "deploy/env/profile-archive.env"
  "deploy/env/profile-bearhost-prod.env"
)
docker_args=(
  --rm
  --network "${network}"
  -v "${ROOT}:/src:ro"
  -w /src
  -v "${secret_dir}:/run/streamclone-secrets:ro"
  -e DATABASE_URL=postgres://app:app@postgres:5432/streamclone?sslmode=disable
  -e REDIS_URL=redis://redis:6379/0
)

for env_file in "${env_files[@]}"; do
  if [[ -f "${env_file}" ]]; then
    docker_args+=(--env-file "${env_file}")
  fi
done

docker run "${docker_args[@]}" "${BEARHOST_GO_IMAGE:-golang:1.25-alpine}" go run "$@"
