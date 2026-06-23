#!/usr/bin/env bash
# BearHost headless corpus mode — bronze + silver + scraper only; stops playback/UI stack.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

# shellcheck source=scripts/lib/bearhost-compose.sh
source "${ROOT}/scripts/lib/bearhost-compose.sh"
# shellcheck source=scripts/bearhost-corpus-preflight.sh
source "${ROOT}/scripts/bearhost-corpus-preflight.sh"

PLAYBACK_STOP=(
  video mediamtx chat emote minio frontend caddy
)

CORPUS_KEEP=(
  postgres redis migrate metadata analytics analytics-workers scraper
)

echo "==> bearhost-corpus-only: stop observability (if running)"
bearhost_compose_obs stop prometheus grafana 2>/dev/null || true

echo "==> bearhost-corpus-only: stop playback + public UI"
bearhost_compose stop "${PLAYBACK_STOP[@]}" 2>/dev/null || true

echo "==> bearhost-corpus-only: corpus preflight"
if bearhost_corpus_preflight; then
  export CORPUS_WORKERS_ENABLED=1
  echo "preflight PASS — CORPUS_WORKERS_ENABLED=1"
else
  export CORPUS_WORKERS_ENABLED=0
  echo "preflight FAIL — workers stay corpus-off (fix Azure secret + Twitch creds)" >&2
fi

echo "==> bearhost-corpus-only: ensure corpus services up"
bearhost_compose up -d "${CORPUS_KEEP[@]}"

echo "==> bearhost-corpus-only: recreate analytics-workers (pick up corpus env)"
bearhost_compose up -d --force-recreate --no-deps analytics-workers

echo ""
echo "==> running corpus services"
bearhost_compose ps "${CORPUS_KEEP[@]}"

echo ""
echo "==> stopped (playback / UI)"
for svc in "${PLAYBACK_STOP[@]}"; do
  cid="$(bearhost_compose ps -q "${svc}" 2>/dev/null || true)"
  if [[ -n "${cid}" ]]; then
    state="$(docker inspect -f '{{.State.Status}}' "${cid}" 2>/dev/null || echo unknown)"
    echo "  ${svc}: ${state}"
  else
    echo "  ${svc}: not running"
  fi
done

echo ""
echo "bearhost-corpus-only: done"
echo "  bronze/silver status: bash scripts/bearhost-bronze-status.sh"
echo "  restore full site:    bash scripts/bearhost-deploy-phased.sh"
