#!/usr/bin/env bash
# Verify Streamclone MCP prerequisites and stdio handshakes (run from repo root).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
PY="$ROOT/.codegraph/.venv/bin/python"
DB="$ROOT/.codegraph/streamclone.kuzu"
FAIL=0

check() {
  if "$@"; then
    echo "  ok: $*"
  else
    echo "  FAIL: $*"
    FAIL=1
  fi
}

mcp_handshake() {
  local label="$1"
  local script="$2"
  local init='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"preflight","version":"1.0"}}}'
  local note='{"jsonrpc":"2.0","method":"notifications/initialized"}'
  local list='{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
  local err="/tmp/streamclone-mcp-preflight-${label}.err"
  local out
  out=$(printf '%s\n%s\n%s\n' "$init" "$note" "$list" | timeout 20 bash "$ROOT/$script" 2>"$err" | tail -1)
  local count
  count=$(printf '%s' "$out" | "$PY" -c "import json,sys; d=json.load(sys.stdin); print(len(d.get('result',{}).get('tools',[])))" 2>/dev/null || echo 0)
  local stderr_preview=""
  if [[ -s "$err" ]]; then
    stderr_preview=$(head -c 200 "$err")
  fi
  if [[ "$count" -gt 0 && -z "$stderr_preview" ]]; then
    echo "  ok: $label handshake ($count tools)"
  else
    echo "  FAIL: $label handshake tools=$count stderr=${stderr_preview:-empty}"
    FAIL=1
  fi
}

echo "Streamclone MCP preflight"
echo "repo: $ROOT"

check test -x "$PY"
check test -e "$DB"
check test -f "$ROOT/scripts/codegraph-mcp.sh"
check test -f "$ROOT/scripts/stack-mcp.sh"
check test -f "$ROOT/scripts/data-mcp.sh"

echo "imports:"
"$PY" - <<'PY'
import importlib
mods = ["mcp", "kuzu", "psycopg", "redis"]
for name in mods:
    importlib.import_module(name)
    print(f"  ok: {name}")
PY

echo "MCP stdio handshakes:"
mcp_handshake codegraph scripts/codegraph-mcp.sh
mcp_handshake stack scripts/stack-mcp.sh
mcp_handshake data scripts/data-mcp.sh

if [[ "$FAIL" -ne 0 ]]; then
  echo "Preflight FAILED"
  exit 1
fi
echo "Preflight passed — reload Cursor and enable MCP servers in Settings."
