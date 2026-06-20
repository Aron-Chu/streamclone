#!/usr/bin/env bash
# Tear down every Streamclone compose overlay (core, scraper, pulse, prod, base).
set -euo pipefail

ENV_FILE="${ENV_FILE:-.env}"
VOLUMES=false

for arg in "$@"; do
  case "$arg" in
    --volumes) VOLUMES=true ;;
    *)
      echo "usage: $0 [--volumes]" >&2
      exit 2
      ;;
  esac
done

if [[ "$VOLUMES" == true ]]; then
  echo "Stopping stacks and removing named volumes (pg-data, minio-data, influx-data, grafana-data)..."
  volume_flag=-v
else
  echo "Stopping all Streamclone compose stacks..."
  volume_flag=
fi

down_stack() {
  # shellcheck disable=SC2086
  docker compose --env-file "${ENV_FILE}" "$@" down --remove-orphans ${volume_flag:+$volume_flag} --timeout 30 || true
}

# Core + scraper + compose pulse stack.
down_stack -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml \
  --profile scraper --profile pulse
down_stack -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml --profile pulse
down_stack -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml
# Prod overlay requires APP_DOMAIN even for `down`.
APP_DOMAIN="${APP_DOMAIN:-streamclone.example.invalid}" \
  ACME_EMAIL="${ACME_EMAIL:-security@example.invalid}" \
  down_stack -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml
down_stack -f deploy/docker-compose.yml

docker rm -f streamclone-chat-tunnel 2>/dev/null || true

if [[ "$VOLUMES" == true ]]; then
  echo "Done."
else
  echo "Done. Run 'make ps' to verify nothing is still listening on app ports."
fi
