#!/usr/bin/env bash
set -euo pipefail

HELPER="/usr/local/sbin/streamclone-laptopworker-power"
if [ ! -x "$HELPER" ]; then
  echo "Power helper not installed. Run: scripts\\laptopworker-remote.cmd setup" >&2
  exit 1
fi
exec sudo -n "$HELPER" "$@" 2>/dev/null || exec sudo "$HELPER" "$@"
