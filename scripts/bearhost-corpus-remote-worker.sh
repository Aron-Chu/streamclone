#!/usr/bin/env bash
# Start BearHost remote silver corpus worker (streampulse-vps SoT over Tailscale).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

ENV_LOCAL="deploy/env/profile-bearhost-corpus-remote-worker.local.env"
if [[ ! -f "${ENV_LOCAL}" ]]; then
  echo "missing ${ENV_LOCAL} — copy from profile-bearhost-corpus-remote-worker.env.example" >&2
  exit 1
fi

# shellcheck source=scripts/lib/bearhost-compose.sh
source "${ROOT}/scripts/lib/bearhost-compose.sh"

VPS_IP="$(grep -E '^STREAMPULSE_VPS_TAILNET_IP=' "${ENV_LOCAL}" | head -1 | cut -d= -f2- | tr -d '\r' || true)"
if [[ -z "${VPS_IP}" ]]; then
  VPS_IP="100.67.17.74"
fi
echo "==> preflight: VPS tailnet postgres ${VPS_IP}:5432"
if ! timeout 5 bash -c "echo >/dev/tcp/${VPS_IP}/5432" 2>/dev/null; then
  echo "BLOCKED: cannot reach ${VPS_IP}:5432 — run streampulse-vps-production-tailnet-db.sh on VPS first" >&2
  exit 1
fi

echo "==> start BearHost remote silver worker"
docker compose \
  --project-name streamclone-bearhost-remote-worker \
  --env-file "${ENV_LOCAL}" \
  -f deploy/docker-compose.bearhost-corpus-remote-worker.yml \
  up -d --build

docker compose \
  --project-name streamclone-bearhost-remote-worker \
  -f deploy/docker-compose.bearhost-corpus-remote-worker.yml \
  ps

echo "bearhost-corpus-remote-worker: done (silver drain; rollback API stack unchanged)"
