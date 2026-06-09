#!/usr/bin/env bash
set -euo pipefail
export PYTHONUNBUFFERED=1
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VENV_PY="$ROOT/.codegraph/.venv/bin/python"
INGEST_PY="$ROOT/tools/codegraph/codegraph_mcp.py"
DB="$ROOT/.codegraph/streamclone.kuzu"

if [[ ! -x "$VENV_PY" ]]; then
  echo "Codegraph venv missing. Run: make codegraph-install" >&2
  exit 1
fi
if [[ ! -e "$DB" ]]; then
  echo "Codegraph database missing. Run: make codegraph" >&2
  exit 1
fi

exec "$VENV_PY" "$INGEST_PY" --repo "$ROOT" --db "$DB"
