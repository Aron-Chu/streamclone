#!/usr/bin/env bash
# Mirror .cursor/skills/streamclone → .agents/skills/streamclone for Codex discovery.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/.cursor/skills/streamclone"
DST="$ROOT/.agents/skills/streamclone"
if [[ ! -d "$SRC" ]]; then
  echo "missing $SRC" >&2
  exit 1
fi
mkdir -p "$DST"
rsync -a --delete "$SRC/" "$DST/"
echo "synced skills → .agents/skills/streamclone"
