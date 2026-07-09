#!/usr/bin/env bash
# Mirror skills for Codex discovery:
#   .cursor/skills/streamclone → .agents/skills/streamclone
# Pulse/backend skills live in streamclone-pulse / streampulse-backend — not synced here.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

sync_dir() {
  local src="$1"
  local dst="$2"
  local label="$3"
  if [[ ! -d "$src" ]]; then
    echo "skip $label (missing $src)" >&2
    return 0
  fi
  mkdir -p "$dst"
  rsync -a --delete "$src/" "$dst/"
  echo "synced $label → $dst"
}

sync_dir "$ROOT/.cursor/skills/streamclone" "$ROOT/.agents/skills/streamclone" "streamclone skills"
