#!/usr/bin/env bash
# Local security checks aligned with CI (gitleaks + env validation).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "==> gitleaks"
if command -v gitleaks >/dev/null 2>&1; then
  gitleaks detect --source . --config .gitleaks.toml --verbose --redact
elif command -v pre-commit >/dev/null 2>&1; then
  pre-commit run gitleaks --all-files
else
  echo "Install gitleaks or pre-commit (make install-hooks)" >&2
  exit 1
fi

echo "==> validate-env"
bash scripts/validate-env.sh

echo "==> local debug instrumentation"
if rg -n "127\\.0\\.0\\.1:7829|X-Debug-Session-Id|#region agent log" frontend internal cmd clipper deploy .github --glob '!frontend/node_modules/**' --glob '!frontend/dist/**' --glob '!**/testdata/rapid/**'; then
  echo "Local debug ingest instrumentation found; remove it before committing." >&2
  exit 1
fi

echo "==> tracked artifact denylist"
bad_paths=()
while IFS= read -r path; do
  case "$path" in
    .kiro/settings/*|deploy/cookies.txt|runtime/clipper-twitch.env|out.json|test.md|pw-*.png|analytics|tmp-vod-*|tmp-metadata-*|debug-*.log|.cursor/debug-*.log|*/testdata/rapid/*.fail)
      bad_paths+=("$path")
      ;;
  esac
done < <(git ls-files)
if [ "${#bad_paths[@]}" -gt 0 ]; then
  printf '%s\n' "${bad_paths[@]}"
  echo "Tracked local artifacts found; remove or untrack them before committing." >&2
  exit 1
fi

echo "security-scan ok"
