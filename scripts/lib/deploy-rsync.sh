#!/usr/bin/env bash
# Shared production deploy/rsync guards and excludes.
# shellcheck shell=bash

require_clean_deploy_tree() {
  local root="$1"
  if [[ "${ALLOW_DIRTY:-}" == "1" ]]; then
    if [[ -z "${ALLOW_DIRTY_REASON:-}" ]]; then
      echo "FAIL: ALLOW_DIRTY=1 requires ALLOW_DIRTY_REASON (e.g. ALLOW_DIRTY_REASON='emergency hotfix')" >&2
      exit 1
    fi
    echo "WARN: ALLOW_DIRTY=1 — deploying from dirty working tree" >&2
    echo "WARN: reason: ${ALLOW_DIRTY_REASON}" >&2
    return 0
  fi
  if [[ -n "$(git -C "${root}" status --porcelain)" ]]; then
    echo "FAIL: working tree dirty; commit/stash or set ALLOW_DIRTY=1 with ALLOW_DIRTY_REASON" >&2
    git -C "${root}" status --short >&2
    exit 1
  fi
}

deploy_rsync_excludes() {
  RSYNC_EXCLUDES=(
    --exclude .git
    --exclude node_modules
    --exclude frontend/node_modules
    --exclude .env
    --exclude .env.local
    --exclude runtime
    --exclude pg-data
    --exclude .codegraph
    --exclude scraper.env
    --exclude 'deploy/env/*.local.env'
    --exclude .cursor/mcp.json
    --exclude .playwright-mcp/
    --exclude '*.wip.bak'
    --exclude scripts/tmp/
  )
}

record_deployed_sha() {
  local root="$1"
  local remote_app="$2"
  local ssh_target="$3"
  local ssh_key="$4"
  local sha
  sha="$(git -C "${root}" rev-parse HEAD)"
  ssh -i "${ssh_key}" -o BatchMode=yes "${ssh_target}" \
    "printf '%s\n' '${sha}' > '${remote_app}/DEPLOYED_SHA' && date -u +%Y-%m-%dT%H:%M:%SZ > '${remote_app}/DEPLOYED_AT'"
  echo "deploy-rsync: recorded DEPLOYED_SHA=${sha} on ${ssh_target}:${remote_app}"
}
