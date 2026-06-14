#!/usr/bin/env bash
# Build helm -f args: chart defaults + optional local overrides.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
chart="${HELM_CHART:-${root}/charts/pulse}"
local_values="${HELM_LOCAL_VALUES:-${root}/deploy/env/helm-local.yaml}"
example_values="${HELM_EXAMPLE_VALUES:-${root}/deploy/env/helm-local.example.yaml}"

args=(-f "${chart}/values.yaml")
if [ -f "$local_values" ]; then
  args+=(-f "$local_values")
elif [ -f "$example_values" ]; then
  args+=(-f "$example_values")
fi
printf '%s\n' "${args[@]}"
