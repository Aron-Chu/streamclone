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

if ! command -v go >/dev/null 2>&1; then
  if command -v docker >/dev/null 2>&1; then
    mount_root="$ROOT"
    if mount_win="$(cd "$ROOT" && pwd -W 2>/dev/null)"; then
      mount_root="$mount_win"
    fi
    pkg_args=""
    for pkg in "${packages[@]}"; do
      pkg_args+=" $(printf %q "$pkg")"
    done
    MSYS_NO_PATHCONV=1 docker run --rm -v "${mount_root}:/src" -w /src golang:1.25-alpine sh -c "
      set -euo pipefail
      staged_go=\$(git diff --cached --name-only --diff-filter=ACM | grep '\\.go\$' || true)
      if [ -n \"\$staged_go\" ]; then
        echo \"\$staged_go\" | xargs gofmt -w
        echo \"\$staged_go\" | xargs git add
      fi
      for pkg in${pkg_args}; do
        if [ -d \"\$pkg\" ]; then
          go vet \"\$pkg/...\"
          go test \"\$pkg/...\"
        fi
      done
    "
    exit $?
  fi
  echo "go not in PATH and docker unavailable; skipping go-fmt-vet hook" >&2
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
