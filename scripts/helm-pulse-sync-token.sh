#!/usr/bin/env bash
# Align k8s/Grafana Influx token with the token InfluxDB PVC actually accepts.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

HELM_NAMESPACE="${HELM_NAMESPACE:-streamclone}"
HELM_RELEASE="${HELM_RELEASE:-streamclone-pulse}"
PULSE_INFLUX_LOCAL_PORT="${PULSE_INFLUX_LOCAL_PORT:-18086}"
SECRET_NAME="${HELM_RELEASE}-secrets"
INFLUX_ORG="${PULSE_INFLUX_ORG:-streamclone}"
INFLUX_BUCKET="${PULSE_INFLUX_BUCKET:-streamclone}"
LEGACY_TOKEN="change-me-influx-token"

influx_write_probe() {
  local try_token="$1"
  local code
  code="$(docker run --rm curlimages/curl:8.5.0 -s -m 5 -o /dev/null -w '%{http_code}' \
    -XPOST "http://host.docker.internal:${PULSE_INFLUX_LOCAL_PORT}/api/v2/write?org=${INFLUX_ORG}&bucket=${INFLUX_BUCKET}&precision=s" \
    -H "Authorization: Token ${try_token}" \
    -H 'Content-Type: text/plain' \
    --data-binary 'stream_activity_1m,channel_login=probe,stream_id=probe chat_count=0i 1' 2>/dev/null || echo "000")"
  [ "$code" = "204" ] || [ "$code" = "200" ]
}

influx_query_code() {
  local try_token="$1"
  docker run --rm curlimages/curl:8.5.0 -s -m 5 -o /dev/null -w '%{http_code}' \
    "http://host.docker.internal:${PULSE_INFLUX_LOCAL_PORT}/api/v2/query?org=${INFLUX_ORG}" \
    -H "Authorization: Token ${try_token}" \
    -H "Content-Type: application/vnd.flux" \
    --data-binary 'from(bucket:"streamclone") |> range(start: -1h) |> limit(n: 1)' 2>/dev/null || echo "000"
}

if ! helm status "$HELM_RELEASE" -n "$HELM_NAMESPACE" >/dev/null 2>&1; then
  exit 0
fi

secret_token="$(kubectl -n "$HELM_NAMESPACE" get secret "$SECRET_NAME" \
  -o jsonpath='{.data.influxdb-token}' 2>/dev/null | base64 -d 2>/dev/null || true)"

secret_query_code="$(influx_query_code "$secret_token")"
legacy_query_code="$(influx_query_code "$LEGACY_TOKEN")"

if [ "$secret_query_code" = "200" ]; then
  echo "Influx token OK (Grafana secret matches PVC)."
  exit 0
fi

working_token=""
if influx_write_probe "$LEGACY_TOKEN" && [ "$legacy_query_code" = "200" ]; then
  working_token="$LEGACY_TOKEN"
elif [ -n "$secret_token" ] && influx_write_probe "$secret_token"; then
  working_token="$secret_token"
fi

if [ -z "$working_token" ]; then
  echo "WARN: Could not find a working Influx token (secret query HTTP ${secret_query_code})." >&2
  exit 0
fi

if [ "$working_token" = "$secret_token" ]; then
  exit 0
fi

echo "Syncing Grafana Influx token to PVC token (${working_token})..."
bash scripts/helm-pulse-patch-secret.sh "$HELM_NAMESPACE" "$SECRET_NAME" "$working_token"
kubectl -n "$HELM_NAMESPACE" rollout restart deployment/"${HELM_RELEASE}-grafana" >/dev/null 2>&1 || true
echo "Grafana datasource token synced (secret patch). Run 'make helm-pulse-wire' if .env INFLUXDB_TOKEN still differs."
