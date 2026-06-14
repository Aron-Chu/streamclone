#!/usr/bin/env bash
# Patch Influx token in the Helm-managed secret (no full helm upgrade).
set -euo pipefail

namespace="${1:?namespace}"
secret_name="${2:?secret name}"
token="${3:?token}"

kubectl -n "$namespace" patch secret "$secret_name" \
  --type merge \
  -p "{\"stringData\":{\"influxdb-token\":\"${token}\"}}"
