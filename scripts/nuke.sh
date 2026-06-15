#!/usr/bin/env bash
# Tear down every Streamclone surface: compose (with volumes), Helm pulse, integration test stack.
set -euo pipefail

ENV_FILE="${ENV_FILE:-.env}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "=== Nuking Streamclone ==="

echo "==> Compose stacks + volumes"
ENV_FILE="${ENV_FILE}" bash scripts/compose-down.sh --volumes

if command -v helm >/dev/null 2>&1 && kubectl config current-context >/dev/null 2>&1; then
  echo "==> Helm release streamclone-pulse (namespace streamclone)"
  helm uninstall streamclone-pulse -n streamclone 2>/dev/null || true
else
  echo "==> Skipping Helm (no cluster context)"
fi

if [ -f internal/integration/docker-compose.test.yml ]; then
  echo "==> Integration test stack"
  docker compose -f internal/integration/docker-compose.test.yml down -v 2>/dev/null || true
fi

if ids="$(docker ps -aq --filter name=streamclone 2>/dev/null)"; then
  if [ -n "${ids}" ]; then
    echo "==> Remaining streamclone containers"
    docker rm -f ${ids} 2>/dev/null || true
  fi
fi

docker rm -f streamclone-chat-tunnel 2>/dev/null || true

echo "Nuke complete. Run 'make ps' to verify nothing is left."
