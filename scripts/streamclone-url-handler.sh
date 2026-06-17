#!/usr/bin/env bash
# streamclone:// URL handler entry — starts setup-control without -RequireProxy (fast wake).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec bash "$(dirname "$0")/ensure-setup-control.sh" "$ROOT"
