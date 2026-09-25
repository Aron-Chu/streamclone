#!/usr/bin/env bash
# Block production topology leaks in the public streamclone repo.
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "${ROOT}"

# Generic, value-free rules. The exact operator deny-list is supplied at run
# time by private streampulse-ops through STREAMPULSE_PRIVATE_TOPOLOGY_PATTERN
# (an extended regex) or STREAMPULSE_PRIVATE_TOPOLOGY_PATTERN_FILE (one
# alternative per line) and is never committed to this public repository.
# The key header is joined at runtime to avoid detect-private-key hook FPs.
_fp='SHA256:[A-Za-z0-9+/=]{20,}'
_openssh_hdr="$(printf '%s%s%s' 'BEGIN OPEN' 'SSH PRIVATE ' 'KEY')"
_private_lan_ip='192\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}'
_named_key='id_(rsa|dsa|ecdsa|ed25519)_[A-Za-z0-9][A-Za-z0-9_-]*'
_root_login='root@[A-Za-z0-9][A-Za-z0-9.-]*[A-Za-z0-9]'
_etc_env='/etc/[A-Za-z0-9._-]+/[A-Za-z0-9._-]*\.env'
PATTERN="${_fp}|${_openssh_hdr}|${_private_lan_ip}|${_named_key}|${_root_login}|${_etc_env}"

PRIVATE_PATTERN="${STREAMPULSE_PRIVATE_TOPOLOGY_PATTERN:-}"
if [[ -z "${PRIVATE_PATTERN}" && -n "${STREAMPULSE_PRIVATE_TOPOLOGY_PATTERN_FILE:-}" ]]; then
  if [[ ! -r "${STREAMPULSE_PRIVATE_TOPOLOGY_PATTERN_FILE}" ]]; then
    echo "public ops guard FAILED: STREAMPULSE_PRIVATE_TOPOLOGY_PATTERN_FILE is not readable" >&2
    exit 2
  fi
  PRIVATE_PATTERN="$(tr -d '\r' <"${STREAMPULSE_PRIVATE_TOPOLOGY_PATTERN_FILE}" | grep -v -e '^[[:space:]]*$' -e '^#' | paste -sd '|' - || true)"
  if [[ -z "${PRIVATE_PATTERN}" ]]; then
    echo "public ops guard FAILED: STREAMPULSE_PRIVATE_TOPOLOGY_PATTERN_FILE has no patterns" >&2
    exit 2
  fi
fi
if [[ -n "${PRIVATE_PATTERN}" ]]; then
  PATTERN="${PATTERN}|${PRIVATE_PATTERN}"
  private_state="private deny-list applied"
else
  private_state="private deny-list not configured; generic rules only"
  if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
    echo "::warning::public ops guard ran without STREAMPULSE_PRIVATE_TOPOLOGY_PATTERN; generic rules only"
  fi
fi

mapfile -t FILES < <(git diff --cached --name-only --diff-filter=ACM)

violations=0
for f in "${FILES[@]}"; do
  case "${f}" in
    # Only the files that define the rules; archives and examples are scanned.
    scripts/pre-commit-public-ops-guard.sh|scripts/ci-public-topology-scan.sh|.cursor/rules/public-repo-boundary.mdc)
      continue
      ;;
  esac
  # Scan the staged blob, not the working tree, and never print its contents.
  set +e
  hits="$(git grep --cached -I -n -E -e "${PATTERN}" -- "${f}" 2>/dev/null)"
  rc=$?
  set -e
  if [[ "${rc}" -ge 2 ]]; then
    echo "public ops guard FAILED: git grep error on ${f}" >&2
    exit 1
  fi
  if [[ "${rc}" -eq 0 ]]; then
    while IFS= read -r hit; do
      rest="${hit#*:}"
      echo "Public ops boundary violation: file=${f} line=${rest%%:*}" >&2
    done <<<"${hits}"
    violations=1
  fi
done

if [[ "${violations}" -ne 0 ]]; then
  echo "Move operator runbooks and host topology to private streampulse-ops." >&2
  exit 1
fi

exit 0
