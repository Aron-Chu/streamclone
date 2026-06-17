#!/usr/bin/env bash
# Copy committed Pulse assets from deploy/ into the Helm chart before helm-up.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
src="${root}/deploy/grafana/dashboards"
dst="${root}/charts/pulse/dashboards"

for name in emote-pulse.json streamclone-ops.json; do
  if [[ ! -f "${src}/${name}" ]]; then
    echo "missing dashboard source: ${src}/${name}" >&2
    exit 1
  fi
  cp "${src}/${name}" "${dst}/${name}"
done

echo "Synced Pulse dashboards → charts/pulse/dashboards/"
