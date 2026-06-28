#!/usr/bin/env bash
# BearHost Pulse API mode — extension backend on localhost:8090 (Cloudflare Tunnel target).
# Stops corpus/scraper workers and playback UI; keeps Tier-0 live tracking + emote + BFF.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

# shellcheck source=scripts/lib/bearhost-compose.sh
source "${ROOT}/scripts/lib/bearhost-compose.sh"

PULSE_STOP=(
  analytics-workers scraper video mediamtx chat frontend caddy
)
PULSE_KEEP=(
  postgres redis migrate metadata analytics emote minio pulse-caddy
)

bearhost_compose_pulse() {
  local root
  root="$(bearhost_root_dir)"
  local args=(
    docker compose
    --env-file "${root}/.env"
    --env-file "${root}/deploy/env/profile-full.env"
    --env-file "${root}/deploy/env/profile-bearhost-prod.env"
    --env-file "${root}/deploy/env/profile-bearhost-pulse.env"
  )
  local f
  while IFS= read -r f; do
    args+=("${f}")
  done < <(bearhost_compose_files)
  args+=(-f "${root}/deploy/docker-compose.bearhost-pulse.yml")
  args+=("$@")
  "${args[@]}"
}

if [[ -f /etc/streamclone/secrets/pulse-beta.env ]]; then
  set -a
  # shellcheck disable=SC1091
  source /etc/streamclone/secrets/pulse-beta.env
  set +a
  echo "==> loaded /etc/streamclone/secrets/pulse-beta.env"
fi

echo "==> bearhost-pulse-api: stop corpus + playback stack"
bearhost_compose_pulse --profile scraper stop "${PULSE_STOP[@]}" 2>/dev/null || true
bearhost_compose --profile scraper stop "${PULSE_STOP[@]}" 2>/dev/null || true

echo "==> bearhost-pulse-api: ensure Pulse API services"
bearhost_compose_pulse up -d "${PULSE_KEEP[@]}"

if [[ "${BEARHOST_SKIP_ANALYTICS_DEPLOY_GATE:-}" != "1" ]]; then
  echo "==> bearhost-pulse-api: predeploy gate (migration 000050 required)"
  BEARHOST_ANALYTICS_GATE_LOCAL=1 bash "${ROOT}/scripts/bearhost-analytics-predeploy-gate.sh" || {
    echo "ABORT: analytics recreate blocked — apply migration 000050 first (make migrate)" >&2
    echo "Break-glass: BEARHOST_SKIP_ANALYTICS_DEPLOY_GATE=1 bash scripts/bearhost-pulse-api.sh" >&2
    exit 1
  }
else
  echo "WARN: BEARHOST_SKIP_ANALYTICS_DEPLOY_GATE=1 — skipping predeploy gate" >&2
fi

echo "==> bearhost-pulse-api: recreate analytics (Tier-0 + hosted env)"
bearhost_compose_pulse up -d --force-recreate --no-deps analytics pulse-caddy

echo ""
echo "==> local health (inside VPS)"
if curl -sf http://127.0.0.1:8090/v1/extension/health >/dev/null; then
  curl -s http://127.0.0.1:8090/v1/extension/health
  echo ""
else
  echo "WARN: http://127.0.0.1:8090/v1/extension/health not ready yet" >&2
  bearhost_compose_pulse ps analytics pulse-caddy emote metadata
fi

echo ""
echo "Next: route api.streampulse.stream → http://localhost:8090 in Cloudflare Tunnel"
echo "Test: curl https://api.streampulse.stream/v1/extension/health"
