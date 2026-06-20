#!/usr/bin/env bash
# Opt-in lightweight pre-commit checks — not a substitute for make check-quick.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

STAGED="$(git diff --cached --name-only --diff-filter=ACM)"
if [ -z "$STAGED" ]; then
  exit 0
fi

if echo "$STAGED" | grep -qE '^deploy/'; then
  make compose-config-check
fi

if echo "$STAGED" | grep -qE '^frontend/src/'; then
  if [ ! -d frontend/node_modules ]; then
    echo "frontend/node_modules missing — run: cd frontend && npm ci"
    exit 1
  fi
  cd frontend && npm test
fi

if echo "$STAGED" | grep -qE '^(Makefile|AGENTS\.md|docs/|\.cursor/)'; then
  while IFS= read -r f; do
    case "$f" in
      *.md) git diff --check --cached -- "$f" ;;
    esac
  done <<< "$STAGED"
fi
