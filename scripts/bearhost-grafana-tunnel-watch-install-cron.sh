#!/usr/bin/env bash
# Install WSL cron entry to keep BearHost Grafana SSH tunnel healthy.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
INTERVAL_MIN="${1:-5}"
if ! [[ "$INTERVAL_MIN" =~ ^[0-9]+$ ]] || [[ "$INTERVAL_MIN" -lt 1 || "$INTERVAL_MIN" -gt 60 ]]; then
  echo "Usage: bearhost-grafana-tunnel-watch-install-cron.sh [interval_minutes=5]" >&2
  exit 2
fi

MARKER="# streamclone-grafana-tunnel-watch"
CRON_LINE="*/${INTERVAL_MIN} * * * * cd ${ROOT} && bash scripts/bearhost-grafana-tunnel-watch.sh"
TMP="$(mktemp)"
crontab -l 2>/dev/null | grep -v "${MARKER}" >"${TMP}" || true
printf '%s\n' "${CRON_LINE} ${MARKER}" >>"${TMP}"
crontab "${TMP}"
rm -f "${TMP}"

echo "Installed WSL cron (every ${INTERVAL_MIN} min):"
echo "  ${CRON_LINE} ${MARKER}"
echo "Log: ~/.streamclone/grafana-tunnel-watch.log"
echo ""
if bash "${ROOT}/scripts/bearhost-grafana-tunnel-watch.sh"; then
  echo "Tunnel healthy."
else
  echo "Initial health check failed; cron will retry every ${INTERVAL_MIN} min." >&2
fi
