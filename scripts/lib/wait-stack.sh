#!/usr/bin/env bash
set -euo pipefail

URL="http://localhost:8090/"
TIMEOUT_SEC=300
INTERVAL_SEC=3

while [ $# -gt 0 ]; do
  case "$1" in
    --url)
      URL="$2"
      shift 2
      ;;
    --timeout)
      TIMEOUT_SEC="$2"
      shift 2
      ;;
    --interval)
      INTERVAL_SEC="$2"
      shift 2
      ;;
    --skip-hls)
      shift
      ;;
    http://*|https://*)
      URL="$1"
      shift
      ;;
    [0-9]*)
      if [ -z "${TIMEOUT_SET:-}" ]; then
        TIMEOUT_SEC="$1"
        TIMEOUT_SET=1
      elif [ -z "${INTERVAL_SET:-}" ]; then
        INTERVAL_SEC="$1"
        INTERVAL_SET=1
      fi
      shift
      ;;
    *)
      shift
      ;;
  esac
done

check_url() {
  curl --connect-timeout 2 --max-time 5 -fsS "$1" >/dev/null 2>&1
}

declare -a APP_LABELS=(
  metadata
  video
  chat
  emote
  analytics
)
declare -a APP_URLS=(
  http://localhost:8081/healthz
  http://localhost:8082/healthz
  http://localhost:8083/healthz
  http://localhost:8084/healthz
  http://localhost:8086/healthz
)

echo "Tier 2/3: application services"
deadline=$((SECONDS + TIMEOUT_SEC))
attempt=0
declare -A ready=()
while [ "$SECONDS" -lt "$deadline" ]; do
  attempt=$((attempt + 1))
  for i in "${!APP_LABELS[@]}"; do
    label="${APP_LABELS[$i]}"
    if [ -n "${ready[$label]:-}" ]; then
      continue
    fi
    if check_url "${APP_URLS[$i]}"; then
      ready[$label]=1
      echo "  $label ready (attempt $attempt)"
    fi
  done
  if [ "${#ready[@]}" -eq "${#APP_LABELS[@]}" ]; then
    break
  fi
  if [ $((attempt % 5)) -eq 0 ]; then
    pending=()
    for label in "${APP_LABELS[@]}"; do
      if [ -z "${ready[$label]:-}" ]; then
        pending+=("$label")
      fi
    done
    echo "  waiting for apps: ${pending[*]} (attempt $attempt)"
  fi
  sleep "$INTERVAL_SEC"
done

if [ "${#ready[@]}" -ne "${#APP_LABELS[@]}" ]; then
  pending=()
  for label in "${APP_LABELS[@]}"; do
    if [ -z "${ready[$label]:-}" ]; then
      pending+=("$label")
    fi
  done
  echo "  application services not ready within ${TIMEOUT_SEC}s (pending: ${pending[*]})" >&2
  echo "Try: docker compose --env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml ps" >&2
  exit 1
fi

echo "Tier 3/3: Caddy proxy ($URL)"
deadline=$((SECONDS + TIMEOUT_SEC))
attempt=0
while [ "$SECONDS" -lt "$deadline" ]; do
  attempt=$((attempt + 1))
  if check_url "$URL"; then
    echo "  Caddy proxy ready (attempt $attempt)"
    echo "Streamclone tiered readiness: all required tiers passed"
    exit 0
  fi
  if [ $((attempt % 5)) -eq 0 ]; then
    echo "  waiting for Caddy proxy... (attempt $attempt)"
  fi
  sleep "$INTERVAL_SEC"
done

echo ""
echo "Streamclone did not become ready in time."
echo "Try: docker compose --env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml ps"
echo "See: docs/install-desktop.md"
exit 1
