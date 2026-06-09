#!/usr/bin/env bash
set -euo pipefail
export PYTHONUNBUFFERED=1
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VENV_PY="$ROOT/.codegraph/.venv/bin/python"
DATA_PY="$ROOT/tools/data/data_mcp.py"

if [[ ! -x "$VENV_PY" ]]; then
  echo "Codegraph venv missing. Run: make codegraph-install" >&2
  exit 1
fi

exec "$VENV_PY" "$DATA_PY"
