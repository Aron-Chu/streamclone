#!/usr/bin/env bash
# Patch Influx token in the Helm-managed secret (no full helm upgrade).
set -euo pipefail

namespace="${1:?namespace}"
secret_name="${2:?secret name}"
token="${3:?token}"
release="${secret_name%-secrets}"
configmap_name="${release}-grafana-provisioning"
dashboard_configmap_name="${release}-grafana-dashboards"
influx_org="${PULSE_INFLUX_ORG:-streamclone}"
influx_bucket="${PULSE_INFLUX_BUCKET:-streamclone}"
influx_service_port="${PULSE_INFLUX_SERVICE_PORT:-18086}"
stream_list_days="${PULSE_STREAM_LIST_DAYS:-90}"
emote_proxy_url="${PULSE_EMOTE_PROXY_URL:-http://localhost:8090}"

kubectl -n "$namespace" patch secret "$secret_name" \
  --type merge \
  -p "{\"stringData\":{\"influxdb-token\":\"${token}\"}}"

if kubectl -n "$namespace" get configmap "$configmap_name" >/dev/null 2>&1; then
  patch_payload="$(python3 - "$token" "$release" "$influx_org" "$influx_bucket" "$influx_service_port" <<'PY'
import json
import sys

token, release, org, bucket, service_port = sys.argv[1:6]
datasources = f"""apiVersion: 1
datasources:
  - name: InfluxDB
    type: influxdb
    access: proxy
    isDefault: true
    url: http://{release}-influxdb:{service_port}
    jsonData:
      version: Flux
      organization: "{org}"
      defaultBucket: "{bucket}"
    secureJsonData:
      token: "{token}"
"""
print(json.dumps({"data": {"datasources.yml": datasources}}))
PY
)"
  kubectl -n "$namespace" patch configmap "$configmap_name" \
    --type merge \
    -p "$patch_payload"
fi

dashboard_source="charts/pulse/dashboards/emote-pulse.json"
if [[ -f "$dashboard_source" ]] && kubectl -n "$namespace" get configmap "$dashboard_configmap_name" >/dev/null 2>&1; then
  dashboard_payload="$(python3 - "$dashboard_source" "$stream_list_days" "$emote_proxy_url" <<'PY'
import json
import sys
from pathlib import Path

source, stream_list_days, emote_proxy_url = sys.argv[1:4]
dashboard = Path(source).read_text(encoding="utf-8")
dashboard = dashboard.replace("__STREAM_LIST_DAYS__", stream_list_days)
dashboard = dashboard.replace("__EMOTE_PROXY__", emote_proxy_url)
print(json.dumps({"data": {"emote-pulse.json": dashboard}}))
PY
)"
  kubectl -n "$namespace" patch configmap "$dashboard_configmap_name" \
    --type merge \
    -p "$dashboard_payload"
fi
