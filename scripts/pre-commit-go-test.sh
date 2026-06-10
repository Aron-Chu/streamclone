#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

mapfile -t packages < <(
  git diff --cached --name-only --diff-filter=ACM |
    grep '\.go$' |
    sed 's|^cmd/\([^/]*\)/.*|./cmd/\1|' |
    sed 's|^internal/\([^/]*\)/.*|./internal/\1|' |
    sed 's|^cmd/\([^/]*\)$|./cmd/\1|' |
    sed 's|^internal/\([^/]*\)$|./internal/\1|' |
    sort -u
)

if [ "${#packages[@]}" -eq 0 ]; then
  exit 0
fi

staged_go="$(git diff --cached --name-only --diff-filter=ACM | grep '\.go$' || true)"
if [ -n "$staged_go" ]; then
  echo "$staged_go" | xargs gofmt -w
  echo "$staged_go" | xargs git add
fi

for pkg in "${packages[@]}"; do
  if [ -d "$pkg" ]; then
    go vet "$pkg/..."
    go test "$pkg/..."
  fi
done
