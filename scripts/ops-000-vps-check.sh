#!/usr/bin/env bash
# OPS-000 VPS archive isolation check (run via bearhost_ssh).
set -euo pipefail

echo "==> OPS-000 VPS checks"
echo "--- docker ps (streamclone)"
docker ps --format "table {{.Names}}\t{{.Status}}" | grep -E "streamclone|NAMES" || true

if docker ps --format "{{.Names}}" | grep -q analytics-workers; then
  echo "BLOCK: analytics-workers is running — wait for archive jobs before Pulse deploy"
  docker exec "$(docker ps --format '{{.Names}}' | grep analytics-workers | head -1)" printenv 2>/dev/null \
    | grep -E "CORPUS|BRONZE|SILVER|ARCHIVE|BACKFILL" || true
  exit 2
fi
echo "PASS: analytics-workers not running"

echo "--- deploy tree"
if [[ -d /opt/streamclone/app ]]; then
  echo "APP=/opt/streamclone/app"
  ls /opt/streamclone/app/deploy/env/profile-bearhost-pulse.env 2>/dev/null && \
    grep -E "STREAMCLONE_VERSION|PULSE_MAX_ACTIVE|CORPUS" /opt/streamclone/app/deploy/env/profile-bearhost-pulse.env || true
else
  echo "WARN: /opt/streamclone/app missing — searching"
  find /home -maxdepth 4 -name profile-bearhost-pulse.env 2>/dev/null | head -3
fi

echo "--- analytics env (cap + version)"
docker exec streamclone-analytics-1 printenv STREAMCLONE_VERSION 2>/dev/null || echo "STREAMCLONE_VERSION=(unset)"
docker exec streamclone-analytics-1 printenv PULSE_MAX_ACTIVE_CHANNELS 2>/dev/null || echo "PULSE_MAX_ACTIVE_CHANNELS=(unset)"
docker exec streamclone-analytics-1 printenv PULSE_HOSTED_MODE 2>/dev/null || echo "PULSE_HOSTED_MODE=(unset)"

echo "OPS-000 VPS: safe for Pulse deploy (no analytics-workers)"
