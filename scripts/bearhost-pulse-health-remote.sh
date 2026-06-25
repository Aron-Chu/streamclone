#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"
bearhost_ssh_config
bearhost_ssh bash -s <<'REMOTE'
set -euo pipefail
echo "==> health"
curl -sf http://127.0.0.1:8090/v1/extension/health
echo ""
echo "==> pulse without key (expect 401 if hosted mode + keys set)"
code=$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8090/v1/extension/pulse/channels/xqc)
echo "HTTP $code"
echo "==> analytics pulse env"
docker exec streamclone-analytics-1 printenv | grep PULSE || true
echo "==> pulse-caddy logs (last 5 lines)"
docker logs streamclone-pulse-caddy 2>&1 | tail -5
REMOTE
