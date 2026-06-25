#!/usr/bin/env bash
# Shared laptopworker env + file sync helpers (sourced by bootstrap/update/stack).
set -euo pipefail

_laptopworker_env_root() {
  local here
  here="$(cd "$(dirname "${BASH_SOURCE[1]:-${BASH_SOURCE[0]}}")/.." && pwd)"
  printf '%s\n' "$here"
}

laptopworker_required_files() {
  local root="$1"
  local rel missing=0
  for rel in \
    deploy/docker-compose.laptopworker-dev.yml \
    deploy/env/profile-laptopworker-dev.env \
    scripts/laptopworker-stack.sh \
    scripts/laptopworker-env.sh; do
    if [ ! -f "$root/$rel" ]; then
      echo "missing required file: $root/$rel" >&2
      missing=1
    fi
  done
  return "$missing"
}

laptopworker_sync_files() {
  local src_root="$1"
  local dest_root="$2"
  local rel
  for rel in \
    deploy/docker-compose.laptopworker-dev.yml \
    deploy/env/profile-laptopworker-dev.env \
    docs/laptopworker-dev.md \
    scripts/laptopworker-bootstrap.sh \
    scripts/laptopworker-env.sh \
    scripts/laptopworker-install-service.sh \
    scripts/laptopworker-power-config.sh \
    scripts/laptopworker-remote.cmd \
    scripts/laptopworker-remote.ps1 \
    scripts/laptopworker-stack.sh \
    scripts/laptopworker-update.sh; do
    if [ ! -f "$src_root/$rel" ]; then
      echo "sync source missing: $src_root/$rel" >&2
      return 1
    fi
    mkdir -p "$dest_root/$(dirname "$rel")"
    cp "$src_root/$rel" "$dest_root/$rel"
    case "$rel" in
      *.sh) chmod +x "$dest_root/$rel" ;;
    esac
  done
}

# Merge profile defaults under existing .env.local (user keys win on conflict).
laptopworker_merge_env_local() {
  local root="$1"
  # shellcheck source=lib/env.sh
  source "$root/scripts/lib/env.sh"
  local profile="$root/deploy/env/profile-laptopworker-dev.env"
  local localfile="$root/.env.local"
  local tmp merged
  tmp="$(mktemp)"
  merged="$(mktemp)"

  if [ ! -f "$profile" ]; then
    echo "profile missing: $profile" >&2
    return 1
  fi

  if [ -f "$localfile" ]; then
    env_merge_files "$merged" "$profile" "$localfile"
  else
    cp "$profile" "$merged"
  fi

  secrets_dir="${STREAMCLONE_SECRETS_DIR:-$HOME/.streamclone/secrets}"
  env_set_key "$merged" STREAMCLONE_SECRETS_DIR "$secrets_dir"

  {
    echo "# Streamclone laptopworker overrides (merged; user keys preserved over profile defaults)"
    echo "# Profile source: deploy/env/profile-laptopworker-dev.env"
    grep -v '^#' "$merged" | grep '=' || true
  } >"$tmp"
  mv "$tmp" "$localfile"
  rm -f "$merged"
}

laptopworker_synth_env() {
  local root="$1"
  laptopworker_merge_env_local "$root"
  # shellcheck source=lib/env.sh
  source "$root/scripts/lib/env.sh"
  env_synthesize core "$root/.env"
  printf '%s' core >"$root/.streamclone-profile"
}

laptopworker_stop_storygraph() {
  docker stop streamclone-storygraph-1 2>/dev/null || true
  docker rm streamclone-storygraph-1 2>/dev/null || true
}

laptopworker_compose_up() {
  local root="$1"
  bash "$root/scripts/laptopworker-stack.sh" start
}
