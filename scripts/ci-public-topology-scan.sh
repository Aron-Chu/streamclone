#!/usr/bin/env bash
# Fail closed if production topology leaks appear in any tracked file.
# Preferred search: ripgrep. Fallback: git grep. Missing both => fail.
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "${ROOT}"

PATTERN='141\.11\.243|23\.173\.152|SHA256:[A-Za-z0-9+/=]{20,}|/root/streampulse-ops|/etc/streamclone/pulse\.env|root@streampulse-vps|id_ed25519_bearhost|PULSE_PROBE_SSH_|streampulse-vps-production-deploy|production\.local\.env'

# Files that must mention patterns while defining the boundary (not disclosures).
allowlisted() {
  case "$1" in
    scripts/ci-public-topology-scan.sh|\
    scripts/pre-commit-public-ops-guard.sh|\
    scripts/ops/filter-repo-paths.txt|\
    scripts/ops/filter-repo-replacements.txt|\
    .cursor/rules/public-repo-boundary.mdc|\
    docs/evidence/public-ref-contamination-report-*.md|\
    docs/evidence/*contamination*)
      return 0
      ;;
  esac
  return 1
}

if command -v rg >/dev/null 2>&1; then
  SEARCH_TOOL=rg
  mapfile -t HITS < <(rg -n -H --hidden --no-messages -g '!.git/*' "${PATTERN}" . 2>/dev/null || true)
elif command -v git >/dev/null 2>&1; then
  SEARCH_TOOL=git-grep
  # Repository-native fallback — covers every tracked file including archives.
  mapfile -t HITS < <(git grep -nI -E "${PATTERN}" -- . 2>/dev/null || true)
else
  echo "ci-public-topology-scan: FAIL closed — neither rg nor git available" >&2
  exit 1
fi

violations=0
for hit in "${HITS[@]:-}"; do
  [ -z "${hit}" ] && continue
  file="${hit%%:*}"
  file="${file#./}"
  if allowlisted "${file}"; then
    continue
  fi
  # Also allow docs/evidence contamination reports by prefix
  case "${file}" in
    docs/evidence/public-ref-contamination-report-*) continue ;;
  esac
  echo "${hit}"
  violations=1
done

if [[ "${violations}" -ne 0 ]]; then
  echo "ci-public-topology-scan: FAIL (tool=${SEARCH_TOOL}) — move operator topology to private streampulse-ops" >&2
  exit 1
fi

echo "ci-public-topology-scan: OK (tool=${SEARCH_TOOL}, all tracked files scanned)"
