#!/usr/bin/env bash
# Block production topology leaks in the public streamclone repo (all tracked paths).
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "${ROOT}"

PATTERN='141\.11\.243|23\.173\.152|SHA256:[A-Za-z0-9+/=]{20,}|/root/streampulse-ops|/etc/streamclone/pulse\.env|root@streampulse-vps|id_ed25519_bearhost|PULSE_PROBE_SSH_|streampulse-vps-production-deploy|production\.local\.env'

if ! command -v rg >/dev/null 2>&1 && ! command -v git >/dev/null 2>&1; then
  echo "Public ops boundary: FAIL closed — neither rg nor git available" >&2
  exit 1
fi

mapfile -t FILES < <(git diff --cached --name-only --diff-filter=ACM)

violations=0
for f in "${FILES[@]:-}"; do
  case "${f}" in
    scripts/pre-commit-public-ops-guard.sh|scripts/ci-public-topology-scan.sh|scripts/ops/filter-repo-paths.txt|scripts/ops/filter-repo-replacements.txt|.cursor/rules/public-repo-boundary.mdc|docs/evidence/public-ref-contamination-report-*)
      continue
      ;;
  esac
  [ -f "${f}" ] || continue
  if command -v rg >/dev/null 2>&1; then
    if rg -n -H "${PATTERN}" "${f}" >/tmp/ops-guard-hit.txt 2>/dev/null; then
      echo "Public ops boundary violation in ${f}:" >&2
      cat /tmp/ops-guard-hit.txt >&2
      violations=1
    fi
  else
    if git grep -nI -E "${PATTERN}" -- "${f}" >/tmp/ops-guard-hit.txt 2>/dev/null; then
      echo "Public ops boundary violation in ${f}:" >&2
      cat /tmp/ops-guard-hit.txt >&2
      violations=1
    fi
  fi
done

if [[ "${violations}" -ne 0 ]]; then
  echo "Move operator runbooks and host topology to private streampulse-ops." >&2
  exit 1
fi

exit 0
