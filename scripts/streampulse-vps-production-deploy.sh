#!/usr/bin/env bash
# Deploy StreamPulse production stack on streampulse-vps (API + DB + single corpus worker).
#
# Builds from the rsynced checkout (bearhost-build overlay). To use GHCR images instead,
# set STREAMPULSE_USE_RELEASE_IMAGES=1 and IMAGE_TAG to an immutable tag that contains this branch.
#
# Does NOT change Cloudflare tunnel/DNS — operator cuts over only after smoke passes.
#
# Usage:
#   bash scripts/streampulse-vps-production-deploy.sh
#   SKIP_RSYNC=1 bash scripts/streampulse-vps-production-deploy.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/deploy-rsync.sh
source "${ROOT}/scripts/lib/deploy-rsync.sh"
# shellcheck source=scripts/lib/streampulse-vps-production-compose.sh
source "${ROOT}/scripts/lib/streampulse-vps-production-compose.sh"

WORKER="${WORKER:-23.173.152.156}"
WORKER_APP="${WORKER_APP:-/opt/streamclone/app}"
SKIP_RSYNC="${SKIP_RSYNC:-0}"
CANARY_GOLD_VOD_SEGMENTS="${CANARY_GOLD_VOD_SEGMENTS:-1}"
PREFLIGHT_ONLY="${PREFLIGHT_ONLY:-0}"

streampulse_vps_resolve_worker_key
ssh_worker() { ssh -i "${WORKER_KEY}" -o BatchMode=yes "root@${WORKER}" "$@"; }

if [[ "${SKIP_RSYNC}" != "1" ]]; then
  echo "==> streampulse-vps: rsync local repo"
  require_clean_deploy_tree "${ROOT}"
  deploy_rsync_excludes
  ssh_worker "mkdir -p ${WORKER_APP}"
  rsync -avz --delete "${RSYNC_EXCLUDES[@]}" \
    --exclude .uv-python --exclude dist \
    -e "ssh -i ${WORKER_KEY} -o BatchMode=yes" \
    "${ROOT}/" "root@${WORKER}:${WORKER_APP}/"
  record_deployed_sha "${ROOT}" "${WORKER_APP}" "root@${WORKER}" "${WORKER_KEY}"

  SCRAPER_ROOT="${SCRAPER_ROOT:-$(cd "${ROOT}/../streamclone-scraper" 2>/dev/null && pwd || true)}"
  if [[ -z "${SCRAPER_ROOT}" || ! -d "${SCRAPER_ROOT}" ]]; then
    SCRAPER_ROOT="${SCRAPER_ROOT:-/mnt/c/Users/Aron/streamclone-scraper}"
  fi
  if [[ -d "${SCRAPER_ROOT}" ]]; then
    echo "==> streampulse-vps: rsync streamclone-scraper"
    ssh_worker "mkdir -p /opt/streamclone/streamclone-scraper"
    rsync -avz --exclude .git --exclude node_modules --exclude __pycache__ \
      -e "ssh -i ${WORKER_KEY} -o BatchMode=yes" \
      "${SCRAPER_ROOT}/" "root@${WORKER}:/opt/streamclone/streamclone-scraper/"
  else
    echo "WARN: streamclone-scraper not found — scraper build may fail" >&2
  fi
fi

GOLD_FLAG=false
GQL_CONCURRENCY=1
if [[ "${CANARY_GOLD_VOD_SEGMENTS}" == "1" || "${CANARY_GOLD_VOD_SEGMENTS}" == "true" ]]; then
  GOLD_FLAG=true
  GQL_CONCURRENCY=2
fi

echo "==> preflight: worker-only corpus stack conflicts"
ssh_worker bash -s <<REMOTE
set -euo pipefail
cd ${WORKER_APP}
# shellcheck source=scripts/lib/streampulse-vps-production-compose.sh
source scripts/lib/streampulse-vps-production-compose.sh
if streampulse_vps_corpus_worker_conflicts "${WORKER_APP}"; then
  echo "BLOCKED: old streampulse-vps corpus worker stack detected:" >&2
  streampulse_vps_corpus_worker_conflicts "${WORKER_APP}" >&2
  echo "Stop it before production deploy, e.g.:" >&2
  echo "  cd ${WORKER_APP} && docker compose -f deploy/docker-compose.streampulse-vps-corpus.yml down" >&2
  exit 1
fi
echo "preflight OK — no corpus worker conflicts"
REMOTE

if [[ "${PREFLIGHT_ONLY}" == "1" ]]; then
  echo "preflight-only complete"
  exit 0
fi

echo "==> streampulse-vps: migrate + build production stack (GOLD_VOD_SEGMENTS_ENABLED=${GOLD_FLAG})"
ssh_worker bash -s <<REMOTE
set -euo pipefail
cd ${WORKER_APP}
ENV_LOCAL=$(streampulse_vps_production_env_local)
if [[ ! -f "\${ENV_LOCAL}" ]]; then
  echo "missing \${ENV_LOCAL} — copy from deploy/env/profile-streampulse-vps-production.env.example" >&2
  exit 1
fi

compose() {
  $(streampulse_vps_production_compose_args "${WORKER_APP}")
}

# Ensure canary env on workers only
grep -q '^GOLD_VOD_SEGMENTS_ENABLED=' "\${ENV_LOCAL}" && \
  sed -i "s/^GOLD_VOD_SEGMENTS_ENABLED=.*/GOLD_VOD_SEGMENTS_ENABLED=${GOLD_FLAG}/" "\${ENV_LOCAL}" || \
  echo "GOLD_VOD_SEGMENTS_ENABLED=${GOLD_FLAG}" >> "\${ENV_LOCAL}"
grep -q '^ANALYTICS_VOD_GQL_CONCURRENCY=' "\${ENV_LOCAL}" && \
  sed -i "s/^ANALYTICS_VOD_GQL_CONCURRENCY=.*/ANALYTICS_VOD_GQL_CONCURRENCY=${GQL_CONCURRENCY}/" "\${ENV_LOCAL}" || \
  echo "ANALYTICS_VOD_GQL_CONCURRENCY=${GQL_CONCURRENCY}" >> "\${ENV_LOCAL}"
grep -q '^BACKFILL_GOLD_WORKER_COUNT=' "\${ENV_LOCAL}" && \
  sed -i 's/^BACKFILL_GOLD_WORKER_COUNT=.*/BACKFILL_GOLD_WORKER_COUNT=1/' "\${ENV_LOCAL}" || \
  echo 'BACKFILL_GOLD_WORKER_COUNT=1' >> "\${ENV_LOCAL}"

echo "==> migrate (000001–000058 chain must be on disk)"
compose up -d postgres redis
sleep 5
compose up migrate
compose wait migrate 2>/dev/null || sleep 10

echo "==> build + start API + single corpus worker (local checkout build)"
compose build analytics analytics-workers scraper emote metadata
compose up -d postgres redis migrate metadata emote analytics pulse-caddy scraper analytics-workers

sleep 20
compose ps
REMOTE

echo "==> local smoke on VPS (127.0.0.1:8090)"
ssh_worker "curl -sf http://127.0.0.1:8090/v1/extension/health | head -c 500; echo"
ssh_worker "curl -sf -o /dev/null -w 'hub30m=%{http_code} %{time_total}s\n' 'http://127.0.0.1:8090/v1/public/hub?activityWindow=30m'"
ssh_worker "curl -sf -o /dev/null -w 'hub7d=%{http_code} %{time_total}s\n' 'http://127.0.0.1:8090/v1/public/hub?activityWindow=7d'"

echo ""
echo "Deploy complete on streampulse-vps. STOP: do not change Cloudflare tunnel until operator approves."
echo "Rollback: keep BearHost API live; docker compose down on VPS if smoke fails."
