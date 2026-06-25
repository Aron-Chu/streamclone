#!/usr/bin/env bash
set -euo pipefail

HELPER="/usr/local/sbin/streamclone-laptopworker-boot"
if [ ! -x "$HELPER" ]; then
  echo "Boot helper not installed. Run: scripts\\laptopworker-remote.cmd setup" >&2
  exit 1
fi
exec sudo -n "$HELPER" "$@" 2>/dev/null || exec sudo "$HELPER" "$@"
