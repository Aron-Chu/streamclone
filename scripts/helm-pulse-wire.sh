#!/usr/bin/env bash
# Wire Docker Compose analytics to the k8s Emote Pulse Influx sandbox.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

ENV_FILE="${ENV_FILE:-.env}"
HELM_NAMESPACE="${HELM_NAMESPACE:-streamclone}"
HELM_RELEASE="${HELM_RELEASE:-streamclone-pulse}"
PULSE_INFLUX_LOCAL_PORT="${PULSE_INFLUX_LOCAL_PORT:-18086}"
PULSE_INFLUX_DOCKER_PORT="${PULSE_INFLUX_DOCKER_PORT:-}"
SECRET_NAME="${HELM_RELEASE}-secrets"
INFLUX_ORG="${PULSE_INFLUX_ORG:-streamclone}"
INFLUX_BUCKET="${PULSE_INFLUX_BUCKET:-streamclone}"

BEGIN="# --- Emote Pulse (make helm-pulse-wire) ---"
END="# --- end Emote Pulse ---"

bash scripts/helm-preflight.sh

if ! helm status "$HELM_RELEASE" -n "$HELM_NAMESPACE" >/dev/null 2>&1; then
  cat >&2 <<EOF
Helm release ${HELM_RELEASE} is not deployed in namespace ${HELM_NAMESPACE}.

Run: make helm-up
EOF
  exit 1
fi

if ! kubectl -n "$HELM_NAMESPACE" get secret "$SECRET_NAME" >/dev/null 2>&1; then
  echo "Secret ${SECRET_NAME} not found in namespace ${HELM_NAMESPACE}." >&2
  exit 1
fi

token="$(kubectl -n "$HELM_NAMESPACE" get secret "$SECRET_NAME" \
  -o jsonpath='{.data.influxdb-token}' | base64 -d)"
if [ -z "$token" ]; then
  echo "Could not read influxdb-token from ${SECRET_NAME}." >&2
  exit 1
fi

echo "Starting port-forwards (Grafana :3000, Influx docker :${PULSE_INFLUX_DOCKER_PORT:-${PULSE_INFLUX_LOCAL_PORT}})..."
HELM_NAMESPACE="$HELM_NAMESPACE" \
  PULSE_INFLUX_LOCAL_PORT="${PULSE_INFLUX_LOCAL_PORT}" \
  PULSE_INFLUX_DOCKER_PORT="${PULSE_INFLUX_DOCKER_PORT:-}" \
  bash scripts/helm-portforward.sh start all

docker_can_reach_url() {
  local url="$1"
  docker run --rm curlimages/curl:8.5.0 -sf -m 3 "${url}/health" >/dev/null 2>&1
}

wsl_primary_ip() {
  hostname -I 2>/dev/null | awk '{print $1}'
}

resolve_influx_url_for_docker() {
  local url port
  for port in "${PULSE_INFLUX_DOCKER_PORT:-}" "${PULSE_INFLUX_LOCAL_PORT}"; do
    [ -n "$port" ] || continue
    if [ -n "${WSL_DISTRO_NAME:-}" ] || grep -qi microsoft /proc/version 2>/dev/null; then
      local ip
      ip="$(wsl_primary_ip)"
      if [ -n "$ip" ]; then
        url="http://${ip}:${port}"
        if docker_can_reach_url "$url"; then
          echo "$url"
          return 0
        fi
      fi
    fi

    for url in \
      "http://host.docker.internal:${port}" \
      "http://gateway.docker.internal:${port}"; do
      if docker_can_reach_url "$url"; then
        echo "$url"
        return 0
      fi
    done
  done
}

influx_url="$(resolve_influx_url_for_docker || true)"
if [ -z "$influx_url" ]; then
  echo "Could not find an Influx URL reachable from Docker containers." >&2
  echo "Run: make helm-influx-stop && make helm-influx" >&2
  exit 1
fi

influx_write_probe() {
  local try_token="$1"
  local docker_code
  docker_code="$(docker run --rm curlimages/curl:8.5.0 -s -m 5 -o /dev/null -w '%{http_code}' \
    -XPOST "${influx_url}/api/v2/write?org=${INFLUX_ORG}&bucket=${INFLUX_BUCKET}&precision=s" \
    -H "Authorization: Token ${try_token}" \
    -H 'Content-Type: text/plain' \
    --data-binary 'stream_activity_1m,channel_login=probe,stream_id=probe chat_count=0i 1' 2>/dev/null || echo "000")"
  [ "$docker_code" = "204" ] || [ "$docker_code" = "200" ]
}

probe_ok=false
if influx_write_probe "$token"; then
  probe_ok=true
fi
if [ "$probe_ok" != "true" ]; then
  legacy_token="change-me-influx-token"
  legacy_ok=false
  if [ "$token" != "$legacy_token" ] && influx_write_probe "$legacy_token"; then
    legacy_ok=true
  fi
  if [ "$legacy_ok" = "true" ]; then
    echo "WARN: Influx PVC still uses legacy token (${legacy_token}); secret has ${token}." >&2
    echo "      Patching secret + restarting Grafana..." >&2
    bash scripts/helm-pulse-patch-secret.sh "$HELM_NAMESPACE" "$SECRET_NAME" "$legacy_token"
    kubectl -n "$HELM_NAMESPACE" rollout restart deployment/"${HELM_RELEASE}-grafana" >/dev/null 2>&1 || true
    token="$legacy_token"
    echo "      Reset for a clean dev token later:" >&2
    echo "        kubectl -n ${HELM_NAMESPACE} delete pvc data-${HELM_RELEASE}-influxdb-0" >&2
    echo "        make helm-up && make helm-pulse-wire" >&2
  else
    cat >&2 <<EOF
Influx write probe failed (401/unreachable) with token from ${SECRET_NAME}.

Ensure port-forward is up: make helm-influx
If token drift after an old install, reset Influx PVC:
  kubectl -n ${HELM_NAMESPACE} delete pvc data-${HELM_RELEASE}-influxdb-0
  make helm-up && make helm-pulse-wire
EOF
    exit 1
  fi
fi

if [ ! -f "$ENV_FILE" ]; then
  if [ -f .env.example ]; then
    bash scripts/env-synthesize.sh core "$ENV_FILE"
    echo "Created ${ENV_FILE} via env-synthesize (core profile)"
  else
    touch "$ENV_FILE"
  fi
fi

block_file="$(mktemp)"
cat >"$block_file" <<EOF
${BEGIN}
TIMESERIES_ENABLED=true
INFLUXDB_URL=${influx_url}
INFLUXDB_TOKEN=${token}
INFLUXDB_ORG=${INFLUX_ORG}
INFLUXDB_BUCKET=${INFLUX_BUCKET}
${END}
EOF

env_tmp="$(mktemp)"
if grep -qF "$BEGIN" "$ENV_FILE"; then
  awk -v begin="$BEGIN" -v end="$END" -v block_file="$block_file" '
    BEGIN {
      while ((getline line < block_file) > 0) block = block line "\n"
      close(block_file)
      inblock = 0
      replaced = 0
    }
    index($0, begin) {
      if (!replaced) { printf "%s", block; replaced = 1 }
      inblock = 1
      next
    }
    inblock {
      if (index($0, end)) inblock = 0
      next
    }
    { print }
  ' "$ENV_FILE" >"$env_tmp"
else
  cat "$ENV_FILE" >"$env_tmp"
  printf '\n' >>"$env_tmp"
  cat "$block_file" >>"$env_tmp"
fi
mv "$env_tmp" "$ENV_FILE"
rm -f "$block_file"

echo "Patched ${ENV_FILE} with Emote Pulse block (TIMESERIES_ENABLED=true, INFLUXDB_*)."
echo "Recreating analytics (and other env-sensitive services)..."
make reload-env ENV_FILE="$ENV_FILE"

cat <<EOF

Emote Pulse wired.

  Grafana:  http://localhost:3000/d/streamclone-emote-pulse/emote-pulse
  Login:    admin / devpulse
  Streamclone analytics → Influx on ${influx_url}

Next: sync chat/emotes in Analytics (http://localhost:8090), then refresh Grafana.
Verify: make pulse-check
EOF
