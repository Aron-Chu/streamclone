#!/usr/bin/env bash
# Launch a Streamclone in-repo MCP server from the git root (WSL/Linux).
# Used by Codex/Cursor on Windows via: wsl.exe --cd <repo> bash scripts/codex-mcp-launch.sh <name>
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
case "${1:-}" in
  codegraph) exec bash "$ROOT/scripts/codegraph-mcp.sh"
  stack)     exec bash "$ROOT/scripts/stack-mcp.sh"
  data)      exec bash "$ROOT/scripts/data-mcp.sh"
  *)
    echo "usage: codex-mcp-launch.sh codegraph|stack|data" >&2
    exit 2
esac
