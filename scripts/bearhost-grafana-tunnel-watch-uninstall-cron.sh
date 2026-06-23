#!/usr/bin/env bash
# Remove WSL cron entry for Grafana tunnel watchdog.
set -euo pipefail
MARKER="# streamclone-grafana-tunnel-watch"
TMP="$(mktemp)"
if ! crontab -l 2>/dev/null | grep -v "${MARKER}" >"${TMP}"; then
  rm -f "${TMP}"
  echo "No grafana tunnel watch cron entry"
  exit 0
fi
crontab "${TMP}"
rm -f "${TMP}"
echo "Removed WSL cron entry (${MARKER})"
