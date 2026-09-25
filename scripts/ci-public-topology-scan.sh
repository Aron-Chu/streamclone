#!/usr/bin/env bash
# Scan every tracked public Streamclone file for production topology / operator
# leak patterns. Guard definitions are allowlisted only by exact path.
#
# Matches are reported by path and line number only; line contents are never
# printed. The exact private deny-list is supplied at run time (see below).
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
_named_key='id_(rsa|dsa|ecdsa|ed25519)_[A-Za-z0-9][A-Za-z0-9_-]*'
_root_login='root@[A-Za-z0-9][A-Za-z0-9.-]*[A-Za-z0-9]'
_etc_env='/etc/[A-Za-z0-9._-]+/[A-Za-z0-9._-]*\.env'
PATTERN="${_fp}|${_openssh_hdr}|${_named_key}|${_root_login}|${_etc_env}"

PRIVATE_PATTERN="${STREAMPULSE_PRIVATE_TOPOLOGY_PATTERN:-}"
if [[ -z "${PRIVATE_PATTERN}" && -n "${STREAMPULSE_PRIVATE_TOPOLOGY_PATTERN_FILE:-}" ]]; then
  if [[ ! -r "${STREAMPULSE_PRIVATE_TOPOLOGY_PATTERN_FILE}" ]]; then
    echo "Public topology scan FAILED: STREAMPULSE_PRIVATE_TOPOLOGY_PATTERN_FILE is not readable" >&2
    exit 2
  fi
  PRIVATE_PATTERN="$(tr -d '\r' <"${STREAMPULSE_PRIVATE_TOPOLOGY_PATTERN_FILE}" | grep -v -e '^[[:space:]]*$' -e '^#' | paste -sd '|' - || true)"
  if [[ -z "${PRIVATE_PATTERN}" ]]; then
    echo "Public topology scan FAILED: STREAMPULSE_PRIVATE_TOPOLOGY_PATTERN_FILE has no patterns" >&2
    exit 2
  fi
fi
if [[ -n "${PRIVATE_PATTERN}" ]]; then
  PATTERN="${PATTERN}|${PRIVATE_PATTERN}"
  private_state="private deny-list applied"
else
  private_state="private deny-list not configured; generic rules only"
  if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
    echo "::warning::Public topology scan ran without STREAMPULSE_PRIVATE_TOPOLOGY_PATTERN; generic rules only"
  fi
fi

ALLOW_FILES=(
  '.cursor/rules/public-repo-boundary.mdc'
  'scripts/ci-public-topology-scan.sh'
  'scripts/pre-commit-public-ops-guard.sh'
)

violations=0
tmp="$(mktemp)"
trap 'rm -f "${tmp}"' EXIT

is_allowlisted_guard_file() {
  local file="$1"
  local allowed
  for allowed in "${ALLOW_FILES[@]}"; do
    [[ "${file}" == "${allowed}" ]] && return 0
  done
  return 1
}

scan_file() {
  local scanner="$1"
  local file="$2"
  local status

  if [[ "${scanner}" == "rg" ]]; then
    if rg -a -n -H -e "${PATTERN}" -- "${file}" >>"${tmp}" 2>/dev/null; then
      return 0
    else
      status=$?
    fi
  else
    if grep -a -E -n -H -e "${PATTERN}" -- "${file}" >>"${tmp}" 2>/dev/null; then
      return 0
    else
      status=$?
    fi
  fi

  case "${status}" in
    1) return 0 ;;
    *)
      echo "Public topology scan FAILED — unable to scan tracked path: ${file}" >&2
      exit 2
      ;;
  esac
}

scanner=""
if [[ "${TOPOLOGY_SCAN_FORCE_GREP:-0}" != "1" ]] && command -v rg >/dev/null 2>&1; then
  scanner="rg"
elif command -v grep >/dev/null 2>&1 && printf 'fallback-check\n' | grep -Eq '^fallback-check$'; then
  scanner="grep"
else
  echo "Public topology scan FAILED — neither rg nor a working grep fallback is available." >&2
  exit 2
fi

# Tracked files plus untracked files that are not ignored, so a file about to
# be committed is caught before it lands. Deleted tracked paths are skipped.
while IFS= read -r -d '' file; do
  [[ -f "${file}" ]] || continue
  scan_file "${scanner}" "${file}"
done < <(git ls-files -z --cached --others --exclude-standard)

if [[ -s "${tmp}" ]]; then
  while IFS= read -r line; do
    file="${line%%:*}"
    if is_allowlisted_guard_file "${file}"; then
      continue
    fi
    rest="${line#*:}"
    echo "topology-hit file=${file} line=${rest%%:*}" >&2
    violations=1
  done <"${tmp}"
fi

if [[ "${violations}" -ne 0 ]]; then
  echo "Public topology scan FAILED — move host IPs / SSH / operator paths to private ops." >&2
  exit 1
fi

echo "ci-public-topology-scan OK (${private_state})"
