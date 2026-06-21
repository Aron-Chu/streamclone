#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/bearhost-ssh.sh
source "${ROOT}/scripts/lib/bearhost-ssh.sh"
bearhost_ssh_config
bearhost_ssh "cd '${BEARHOST_REMOTE_APP}' && BEARHOST_USE_DOCKER_GO=1 bash scripts/bearhost-go-run.sh ./cmd/backfill bronze vod-range --login=xqc"
bearhost_ssh "cd '${BEARHOST_REMOTE_APP}' && BEARHOST_USE_DOCKER_GO=1 bash scripts/bearhost-go-run.sh ./cmd/backfill bronze vod-range --login=sodapoppin"
bearhost_ssh "cd '${BEARHOST_REMOTE_APP}' && BEARHOST_USE_DOCKER_GO=1 bash scripts/bearhost-go-run.sh ./cmd/backfill bronze vod-range --login=cellbit"
bearhost_ssh "cd '${BEARHOST_REMOTE_APP}' && BEARHOST_USE_DOCKER_GO=1 bash scripts/bearhost-go-run.sh ./cmd/backfill bronze vod-range --login=summit1g"
