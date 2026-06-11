#!/usr/bin/env bash
# Purge accidental dev junk from entire git history (binaries, pyc, debug artifacts).
# WARNING: rewrites history — coordinate with all collaborators before force-pushing.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if ! command -v git-filter-repo >/dev/null 2>&1; then
  echo "Install git-filter-repo: pip install git-filter-repo  OR  apt install git-filter-repo"
  exit 1
fi

echo "This will rewrite git history in $ROOT"
echo "Paths removed from all commits:"
cat <<'PATHS'
tmp-metadata-app.bin
tmp-metadata-app-current.bin
**/__pycache__/**
.cursor/mcp.json
frontend/artifacts/**
frontend/playwright-*-debug.png
debug-mcp-*.txt
mcp-*.txt
frontend/vite-oauth.out.log
frontend/vite-oauth.err.log
deploy/tmp-index.m3u8
.vscode/settings.json
PATHS

read -r -p "Continue? [y/N]: " ans
if [[ ! "$ans" =~ ^[Yy]$ ]]; then
  echo "Aborted."
  exit 0
fi

git filter-repo --force \
  --path tmp-metadata-app.bin --invert-paths \
  --path tmp-metadata-app-current.bin --invert-paths \
  --path-glob '**/__pycache__/*' --invert-paths \
  --path .cursor/mcp.json --invert-paths \
  --path-glob 'frontend/artifacts/*' --invert-paths \
  --path-glob 'frontend/playwright-*-debug.png' --invert-paths \
  --path-glob 'debug-mcp-*.txt' --invert-paths \
  --path-glob 'mcp-*.txt' --invert-paths \
  --path frontend/vite-oauth.out.log --invert-paths \
  --path frontend/vite-oauth.err.log --invert-paths \
  --path deploy/tmp-index.m3u8 --invert-paths \
  --path .vscode/settings.json --invert-paths

echo ""
echo "History rewritten locally. Verify with: git log --oneline && git rev-list --objects --all | grep tmp-metadata || true"
echo "To publish: git push --force-with-lease origin master"
echo "All collaborators must re-clone or reset after force-push."
