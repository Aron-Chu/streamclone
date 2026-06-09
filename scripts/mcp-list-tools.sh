#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PY="$ROOT/.codegraph/.venv/bin/python"

mcp_tools_list() {
  local name="$1"
  shift
  local init='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"tools-check","version":"1.0"}}}'
  local note='{"jsonrpc":"2.0","method":"notifications/initialized"}'
  local list='{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
  printf '%s\n%s\n%s\n' "$init" "$note" "$list" | timeout 8 "$@" 2>/tmp/mcp-tools-err-$$.txt | tail -1
}

echo "=== streamclone-codegraph ==="
out=$(mcp_tools_list codegraph "$PY" "$ROOT/tools/codegraph/codegraph_mcp.py" --repo "$ROOT" --db "$ROOT/.codegraph/streamclone.kuzu")
echo "$out" | "$PY" -c "import json,sys; d=json.load(sys.stdin); tools=d.get('result',{}).get('tools',[]); print('tool_count=', len(tools)); print('names=', [t['name'] for t in tools])"

echo "=== streamclone-stack ==="
out=$(mcp_tools_list stack "$PY" "$ROOT/tools/stack/stack_mcp.py" --repo "$ROOT")
echo "$out" | "$PY" -c "import json,sys; d=json.load(sys.stdin); tools=d.get('result',{}).get('tools',[]); print('tool_count=', len(tools)); print('names=', [t['name'] for t in tools])"

echo "=== streamclone-data ==="
out=$(mcp_tools_list data "$PY" "$ROOT/tools/data/data_mcp.py")
echo "$out" | "$PY" -c "import json,sys; d=json.load(sys.stdin); tools=d.get('result',{}).get('tools',[]); print('tool_count=', len(tools)); print('names=', [t['name'] for t in tools])"
