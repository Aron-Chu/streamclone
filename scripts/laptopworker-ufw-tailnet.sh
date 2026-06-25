#!/usr/bin/env bash
# Tailnet-only firewall — uses root-owned /usr/local/sbin helper.
set -euo pipefail

HELPER="/usr/local/sbin/streamclone-laptopworker-firewall"

if [ ! -x "$HELPER" ]; then
  echo "Firewall helper not installed. Run: scripts\\laptopworker-remote.cmd setup" >&2
  exit 1
fi

exec sudo -n "$HELPER" "$@" 2>/dev/null || exec sudo "$HELPER" "$@"
