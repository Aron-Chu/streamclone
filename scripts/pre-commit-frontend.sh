#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/frontend"

if ! git diff --cached --name-only --diff-filter=ACM | grep -qE '^frontend/(src|e2e)/'; then
  exit 0
fi

if [ ! -d node_modules ]; then
  echo "frontend/node_modules missing — run: cd frontend && npm ci"
  exit 1
fi

npm exec tsc -b --pretty false
