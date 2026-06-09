#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
PY="$ROOT/.codegraph/.venv/bin/python"

echo "=== venv ==="
if [[ ! -x "$PY" ]]; then
  echo "MISSING: run make codegraph-install"
  exit 1
fi
echo "ok"

echo "=== codegraph graph_status ==="
if [[ ! -e "$ROOT/.codegraph/streamclone.kuzu" ]]; then
  echo "MISSING graph DB: run make codegraph"
else
  "$PY" - <<'PY'
import json, sys
sys.path.insert(0, "tools/codegraph")
from codegraph_mcp import graph_status
s = graph_status()
print(json.dumps({k: s[k] for k in ("exists", "indexed_at_utc", "counts") if k in s}, indent=2))
PY
fi

echo "=== stack stack_ports ==="
"$PY" - <<'PY'
import json, sys
sys.path.insert(0, "tools/stack")
from stack_mcp import stack_ports
r = stack_ports()
print("port_keys:", sorted(r.get("ports", {}).keys()))
print("hints:", r.get("hints", []))
PY

echo "=== data data_status ==="
"$PY" - <<'PY'
import json, sys
sys.path.insert(0, "tools/data")
from data_mcp import data_status
print(json.dumps(data_status(), indent=2))
PY

echo "=== hooks smoke ==="
echo '{"file_path":"internal/analytics/foo.go"}' | python3 "$ROOT/.cursor/hooks/codegraph-stale-hint.py" || true
