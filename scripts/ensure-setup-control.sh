#!/usr/bin/env bash
# Thin bash wrapper for ensure-setup-control.ps1 (macOS/Linux install and start paths).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="${1:-$(cd "$SCRIPT_DIR/.." && pwd)}"
shift || true

if command -v pwsh >/dev/null 2>&1; then
  exec pwsh -NoProfile -ExecutionPolicy Bypass -File "$SCRIPT_DIR/ensure-setup-control.ps1" -Root "$ROOT" "$@"
fi
if command -v powershell.exe >/dev/null 2>&1; then
  exec powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$SCRIPT_DIR/ensure-setup-control.ps1" -Root "$ROOT" "$@"
fi

echo "PowerShell (pwsh or powershell.exe) is required to start setup-control." >&2
exit 1
