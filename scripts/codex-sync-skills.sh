#!/usr/bin/env bash
# Mirror skills for Codex discovery:
#   .cursor/skills/streamclone → .agents/skills/streamclone
#   .cursor/skills/pulse       → .agents/skills/pulse
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
sync_dir "$ROOT/.cursor/skills/pulse" "$ROOT/.agents/skills/pulse" "pulse skills"

# Optional: refresh helper scripts from sibling streamclone-pulse (SKILL.md stays streamclone-authored).
PULSE_SIBLING="$ROOT/../streamclone-pulse/.cursor/skills"
if [[ -d "$PULSE_SIBLING" ]]; then
  for skill in backfill-safety-review api-contract-drift-check; do
    if [[ -d "$PULSE_SIBLING/$skill/scripts" ]]; then
      mkdir -p "$ROOT/.agents/skills/pulse/$skill/scripts"
      rsync -a "$PULSE_SIBLING/$skill/scripts/" "$ROOT/.agents/skills/pulse/$skill/scripts/"
    fi
  done
  echo "merged pulse helper scripts from sibling streamclone-pulse (if present)"
fi
