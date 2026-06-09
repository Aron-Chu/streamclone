#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOOK="${1:?hook script name under .cursor/hooks/}"
exec python3 "$ROOT/.cursor/hooks/$HOOK"
