#!/usr/bin/env bash
# Verify Emote Pulse k8s sandbox + compose analytics wiring.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

ENV_FILE="${ENV_FILE:-.env}"
HELM_NAMESPACE="${HELM_NAMESPACE:-streamclone}"
HELM_RELEASE="${HELM_RELEASE:-streamclone-pulse}"
PULSE_INFLUX_LOCAL_PORT="${PULSE_INFLUX_LOCAL_PORT:-18086}"
SECRET_NAME="${HELM_RELEASE}-secrets"
INFLUX_ORG="${PULSE_INFLUX_ORG:-streamclone}"
INFLUX_BUCKET="${PULSE_INFLUX_BUCKET:-streamclone}"

COMPOSE=(docker compose --env-file "$ENV_FILE" -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml)

pass=0
fail=0
warn=0

ok() {
  echo "  OK   $*"
  pass=$((pass + 1))
}

bad() {
  echo "  FAIL $*"
  fail=$((fail + 1))
}

note() {
  echo "  WARN $*"
  warn=$((warn + 1))
}

bash scripts/helm-preflight.sh >/dev/null

echo "Emote Pulse check (namespace=${HELM_NAMESPACE}, influx=localhost:${PULSE_INFLUX_LOCAL_PORT})"
echo

echo "Kubernetes pods"
if ! helm status "$HELM_RELEASE" -n "$HELM_NAMESPACE" >/dev/null 2>&1; then
  bad "Helm release ${HELM_RELEASE} not deployed → make helm-up"
else
  for component in influxdb grafana prometheus; do
    line="$(kubectl -n "$HELM_NAMESPACE" get pods -l "app.kubernetes.io/instance=${HELM_RELEASE},app.kubernetes.io/component=${component}" \
      --no-headers 2>/dev/null | head -1 || true)"
    if [ -z "$line" ]; then
      bad "${component} pod missing"
      continue
    fi
    ready="$(echo "$line" | awk '{print $2}')"
    status="$(echo "$line" | awk '{print $3}')"
    if [ "$ready" = "1/1" ] && [ "$status" = "Running" ]; then
      ok "${component} ${ready} ${status}"
    else
      bad "${component} ${ready} ${status} → kubectl -n ${HELM_NAMESPACE} describe pod -l app.kubernetes.io/component=${component}"
    fi
  done
fi

influx_health_url() {
  if curl -sf -m 2 "http://127.0.0.1:${PULSE_INFLUX_LOCAL_PORT}/health" >/dev/null 2>&1; then
    echo "http://127.0.0.1:${PULSE_INFLUX_LOCAL_PORT}"
  elif curl -sf -m 2 "http://host.docker.internal:${PULSE_INFLUX_LOCAL_PORT}/health" >/dev/null 2>&1; then
    echo "http://host.docker.internal:${PULSE_INFLUX_LOCAL_PORT}"
  else
    echo ""
  fi
}

echo
echo "Port-forwards"
influx_base="$(influx_health_url)"
if [ -n "$influx_base" ]; then
  ok "Influx health ${influx_base}/health"
else
  bad "Influx not reachable on :${PULSE_INFLUX_LOCAL_PORT} → make helm-influx"
fi

is_wsl() {
  [ -n "${WSL_DISTRO_NAME:-}" ] || grep -qi microsoft /proc/version 2>/dev/null
}

grafana_code="000"
if is_wsl && command -v curl.exe >/dev/null 2>&1; then
  grafana_code="$(curl.exe -sf -m 3 -o NUL -w '%{http_code}' "http://127.0.0.1:3000/login" 2>/dev/null || echo "000")"
fi
if [ "$grafana_code" != "200" ]; then
  grafana_code="$(curl -sf -o /dev/null -w '%{http_code}' "http://127.0.0.1:3000/login" 2>/dev/null || echo "000")"
fi
if [ "$grafana_code" != "200" ]; then
  grafana_code="$(curl.exe -sf -m 3 -o NUL -w '%{http_code}' "http://127.0.0.1:3000/login" 2>/dev/null || echo "000")"
fi
if [ "$grafana_code" = "200" ]; then
  ok "Grafana login page http://127.0.0.1:3000 (${grafana_code})"
else
  bad "Grafana not reachable on :3000 (HTTP ${grafana_code}) → make helm-grafana"
fi

prometheus_code="$(curl -sf -o /dev/null -w '%{http_code}' "http://127.0.0.1:9090/-/healthy" 2>/dev/null || echo "000")"
if [ "$prometheus_code" = "200" ]; then
  ok "Prometheus healthy http://127.0.0.1:9090"
else
  note "Prometheus not reachable on :9090 (HTTP ${prometheus_code}) — upgrade chart or make helm-up"
fi

analytics_container() {
  docker ps --format '{{.Names}}' 2>/dev/null | grep 'streamclone-analytics' | head -1 || true
}

container_env() {
  local name="$1"
  local key="$2"
  docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$name" 2>/dev/null \
    | grep "^${key}=" | head -1 | cut -d= -f2- | tr -d '[:space:]' || true
}

docker_url_reachable() {
  local url="$1"
  docker run --rm curlimages/curl:8.5.0 -sf -m 3 \
    "${url}/health" >/dev/null 2>&1
}

echo
echo "Compose analytics env"
analytics_c="$(analytics_container)"
if [ -z "$analytics_c" ]; then
  bad "analytics container not running → make up"
else
  ts_enabled="$(container_env "$analytics_c" TIMESERIES_ENABLED)"
  if [ "$ts_enabled" = "true" ]; then
    ok "TIMESERIES_ENABLED=true in analytics container"
  else
    bad "TIMESERIES_ENABLED=${ts_enabled:-<unset>} → make helm-pulse-wire"
  fi

  influx_url="$(container_env "$analytics_c" INFLUXDB_URL)"
  if [ "$influx_url" = "http://host.docker.internal:${PULSE_INFLUX_LOCAL_PORT}" ]; then
    ok "INFLUXDB_URL=${influx_url}"
  elif [ -n "$influx_url" ]; then
    ok "INFLUXDB_URL=${influx_url}"
  else
    bad "INFLUXDB_URL unset → make helm-pulse-wire"
  fi

  echo
  echo "Docker → Influx (${influx_url:-unset})"
  if [ -n "$influx_url" ] && docker_url_reachable "$influx_url"; then
    ok "Container can reach Influx at ${influx_url}"
  else
    bad "Container cannot reach ${influx_url:-<unset>} → make helm-pulse-wire"
  fi
fi

echo
echo "Analytics write path (recent logs)"
if docker ps --format '{{.Names}}' 2>/dev/null | grep -q 'streamclone-analytics'; then
  log_tail="$("${COMPOSE[@]}" logs analytics --tail=30 2>&1 || true)"
  if echo "$log_tail" | grep -q 'time-series write failed'; then
    bad "Recent time-series write failures in analytics logs (last 30 lines) → make helm-influx-stop && make helm-influx, then make reload-env"
  else
    ok "No recent time-series write failed in last 30 log lines"
  fi
else
  note "Skipped log check (analytics not running)"
fi

echo
echo "Influx data (optional, last 24h)"
token=""
if kubectl -n "$HELM_NAMESPACE" get secret "$SECRET_NAME" >/dev/null 2>&1; then
  token="$(kubectl -n "$HELM_NAMESPACE" get secret "$SECRET_NAME" \
    -o jsonpath='{.data.influxdb-token}' 2>/dev/null | base64 -d 2>/dev/null || true)"
fi

flux='from(bucket:"'"${INFLUX_BUCKET}"'") |> range(start: -24h) |> filter(fn: (r) => r._measurement == "stream_activity_1m" or r._measurement == "emote_usage_1m") |> group() |> count()'

influx_query_body() {
  local try_token="$1"
  docker run --rm curlimages/curl:8.5.0 -s -m 5 \
    "${influx_url:-http://host.docker.internal:${PULSE_INFLUX_LOCAL_PORT}}/api/v2/query?org=${INFLUX_ORG}" \
    -H "Authorization: Token ${try_token}" \
    -H "Content-Type: application/vnd.flux" \
    --data-binary "$flux" 2>/dev/null || true
}

influx_query_http() {
  local try_token="$1"
  local code
  code="$(docker run --rm curlimages/curl:8.5.0 -s -m 5 -o /dev/null -w '%{http_code}' \
    "${influx_url:-http://host.docker.internal:${PULSE_INFLUX_LOCAL_PORT}}/api/v2/query?org=${INFLUX_ORG}" \
    -H "Authorization: Token ${try_token}" \
    -H "Content-Type: application/vnd.flux" \
    --data-binary "$flux" 2>/dev/null || echo "000")"
  echo "$code"
}

if [ -z "$token" ]; then
  note "Skipped Flux query (no token from ${SECRET_NAME})"
elif ! docker info >/dev/null 2>&1; then
  note "Skipped Flux query (docker unavailable for Influx probe)"
else
  flux_http="$(influx_query_http "$token")"
  flux_body="$(influx_query_body "$token")"
  if [ "$flux_http" = "401" ]; then
    bad "Grafana Influx token rejected (401) — PVC uses a different token → make helm-pulse-sync-token or make helm-pulse-wire"
  elif [ "$flux_http" != "200" ]; then
    note "Influx Flux query HTTP ${flux_http} (is port-forward up? make pulse-on)"
  elif echo "$flux_body" | grep -qE '_value|[1-9][0-9]*'; then
    ok "Flux found stream_activity_1m or emote_usage_1m in last 24h"
  else
    note "No rollup data in last 24h → sync a stream in Analytics UI (chat/emote rollups)"
  fi
fi

echo
echo "Summary: ${pass} passed, ${fail} failed, ${warn} warnings"
if [ "$fail" -gt 0 ]; then
  exit 1
fi
