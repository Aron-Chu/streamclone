#!/usr/bin/env bash
# Sync Cursor subagent definitions to VS Code Copilot custom agents (.github/agents/*.agent.md).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/.github/agents"
mkdir -p "$OUT"

readonly_tools="['search/codebase', 'search/usages', 'web/fetch', 'streamcloneCodegraph/*', 'streamcloneStack/*', 'streamcloneData/*']"

sync_agent() {
  local src="$1"
  local out_name="$2"
  if [[ ! -f "$src" ]]; then
    echo "skip $out_name (missing $src)" >&2
    return 0
  fi
  python3 - "$src" "$OUT/$out_name" "$readonly_tools" <<'PY'
import re
import sys
from pathlib import Path

src, dst, tools = sys.argv[1], sys.argv[2], sys.argv[3]
text = Path(src).read_text(encoding="utf-8")
if not text.startswith("---"):
    raise SystemExit(f"no frontmatter in {src}")

parts = text.split("---", 2)
if len(parts) < 3:
    raise SystemExit(f"invalid frontmatter in {src}")

body = parts[2].lstrip("\n")
meta_lines = [ln for ln in parts[1].splitlines() if ln.strip()]
fields = {}
for ln in meta_lines:
    if ":" not in ln:
        continue
    key, val = ln.split(":", 1)
    key = key.strip()
    val = val.strip()
    if key in {"readonly", "is_background", "model"}:
        continue
    fields[key] = val

name = fields.get("name") or Path(dst).stem.replace(".agent", "")
description = fields.get("description", "")
out = (
    "---\n"
    f"name: {name}\n"
    f"description: {description}\n"
    f"tools: {tools}\n"
    "---\n"
    f"{body}"
)
Path(dst).write_text(out, encoding="utf-8")
print(f"synced {Path(src).name} -> {Path(dst).name}")
PY
}

sync_agent "$ROOT/.cursor/agents/backend-safety-reviewer.md" "backend-safety-reviewer.agent.md"
sync_agent "$ROOT/.cursor/agents/ops-diagnostics-reviewer.md" "ops-diagnostics-reviewer.agent.md"

PULSE_AGENT="$ROOT/../streamclone-pulse/.cursor/agents/frontend-ux-reviewer.md"
sync_agent "$PULSE_AGENT" "frontend-ux-reviewer.agent.md"

echo "VS Code Copilot agents synced to $OUT"
