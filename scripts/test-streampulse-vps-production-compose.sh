#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/hosted-production-vps-production-compose.sh
source "${ROOT}/scripts/lib/hosted-production-vps-production-compose.sh"

docker() {
  if [[ "${1:-}" != "ps" ]]; then
    echo "unexpected docker invocation: docker $*" >&2
    return 2
  fi

  local has_project_filter=0
  local arg
  for arg in "$@"; do
    if [[ "${arg}" == "label=com.docker.compose.project=streamclone-corpus" ]]; then
      has_project_filter=1
      break
    fi
  done

  case "${DOCKER_FIXTURE:-}" in
    production_only)
      if [[ "${has_project_filter}" == "0" ]]; then
        printf '%s\n' \
          'streampulse-analytics-workers|Up 2 hours' \
          'streampulse-scraper|Up 2 hours'
      fi
      ;;
    legacy_project)
      if [[ "${has_project_filter}" == "1" ]]; then
        printf '%s\n' \
          'streampulse-analytics-workers|Exited (0) 3 hours ago' \
          'streampulse-scraper|Exited (0) 3 hours ago'
      fi
      ;;
    legacy_collectors)
      if [[ "${has_project_filter}" == "0" ]]; then
        printf '%s\n' \
          'streamclone-collector|Exited (1) 4 hours ago' \
          'streamclone-pulse-irc-collector|Up 5 hours'
      fi
      ;;
    *)
      echo "missing or unknown DOCKER_FIXTURE=${DOCKER_FIXTURE:-}" >&2
      return 2
      ;;
  esac
}

run_conflict_check() {
  local fixture="${1:?fixture required}"
  local output status
  if output="$(DOCKER_FIXTURE="${fixture}" streampulse_vps_corpus_worker_conflicts "${ROOT}" 2>&1)"; then
    status=0
  else
    status=$?
  fi
  printf '%s\t%s\t%s\n' "${fixture}" "${status}" "${output}"
}

production_result="$(run_conflict_check production_only)"
legacy_project_result="$(run_conflict_check legacy_project)"
legacy_collectors_result="$(run_conflict_check legacy_collectors)"

if [[ "${production_result}" != $'production_only\t1\t' ]]; then
  echo "production containers should not conflict:" >&2
  echo "${production_result}" >&2
  exit 1
fi

if [[ "${legacy_project_result}" != *$'legacy_project\t0\tstreampulse-analytics-workers (Exited (0) 3 hours ago)'* ]]; then
  echo "legacy streamclone-corpus project should conflict:" >&2
  echo "${legacy_project_result}" >&2
  exit 1
fi

if [[ "${legacy_collectors_result}" != *$'legacy_collectors\t0\tstreamclone-collector (Exited (1) 4 hours ago)'* ]] ||
   [[ "${legacy_collectors_result}" != *'streamclone-pulse-irc-collector (Up 5 hours)'* ]]; then
  echo "legacy collector containers should conflict:" >&2
  echo "${legacy_collectors_result}" >&2
  exit 1
fi

printf '%s\n' "${production_result}" "${legacy_project_result}" "${legacy_collectors_result}"
