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
    scripts/laptopworker-install-firewall-service.sh \
    scripts/laptopworker-install-helpers.sh \
    scripts/laptopworker-install-service.sh \
    scripts/laptopworker-install-sudoers.sh \
    scripts/laptopworker-power-config.sh \
    scripts/laptopworker-remote.cmd \
    scripts/laptopworker-remote.ps1 \
    scripts/laptopworker-setup-boot.sh \
    scripts/laptopworker-setup-remote.sh \
    scripts/laptopworker-stack.sh \
    scripts/laptopworker-update.sh \
    scripts/laptopworker-ufw-tailnet.sh; do
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

laptopworker_ensure_scripts_executable() {
  local root="$1"
  local rel
  for rel in scripts/laptopworker-*.sh; do
    [ -f "$root/$rel" ] || continue
    chmod +x "$root/$rel"
  done
}

# Build docker compose argv for laptopworker core stack (array name: LAPTOPWORKER_COMPOSE).
laptopworker_compose_init() {
  local root="$1"
  LAPTOPWORKER_COMPOSE=(
    docker compose --env-file "$root/.env"
    -f "$root/deploy/docker-compose.yml"
    -f "$root/deploy/docker-compose.local-tunnel.yml"
    -f "$root/deploy/docker-compose.laptopworker-dev.yml"
  )
  if [ -f "$root/deploy/env/.env.pulse-local" ]; then
    LAPTOPWORKER_COMPOSE+=(--env-file "$root/deploy/env/.env.pulse-local")
  fi
  if [ -f "$root/.env.local" ]; then
    LAPTOPWORKER_COMPOSE+=(--env-file "$root/.env.local")
  fi
}

laptopworker_compose() {
  local root="$1"
  shift
  laptopworker_compose_init "$root"
  (cd "$root" && "${LAPTOPWORKER_COMPOSE[@]}" "$@")
}

# Analyze git diff and set globals: LW_BUILD[], LW_RECREATE[], LW_RUN_MIGRATE, LW_FULL_COMPOSE.
laptopworker_plan_update() {
  local root="$1"
  local old_sha="$2"
  local new_sha="$3"
  local changed line svc
  declare -A build_pick=()
  declare -A recreate_pick=()

  LW_BUILD=()
  LW_RECREATE=()
  LW_RUN_MIGRATE=0
  LW_FULL_COMPOSE=0

  if [ "$old_sha" = "$new_sha" ]; then
    return 0
  fi

  changed="$(git -C "$root" diff --name-only "$old_sha" "$new_sha" 2>/dev/null || true)"
  [ -n "$changed" ] || return 0

  while IFS= read -r line; do
    [ -n "$line" ] || continue
    case "$line" in
      go.mod|go.sum)
        build_pick[metadata]=1
        build_pick[video]=1
        build_pick[chat]=1
        build_pick[emote]=1
        ;;
        build_pick[frontend]=1
        ;;
      cmd/*|internal/*|deploy/Dockerfile*)
        build_pick[metadata]=1
        build_pick[video]=1
        build_pick[chat]=1
        build_pick[emote]=1
        ;;
      deploy/docker-compose*|deploy/env/profile-laptopworker*)
        LW_FULL_COMPOSE=1
        ;;
      deploy/Caddyfile*)
        recreate_pick[local-proxy]=1
        build_pick[frontend]=1
        ;;
      deploy/mediamtx.yml)
        recreate_pick[mediamtx]=1
        recreate_pick[video]=1
        ;;
      migrations/*)
        LW_RUN_MIGRATE=1
        build_pick[metadata]=1
        build_pick[video]=1
        build_pick[chat]=1
        build_pick[emote]=1
        ;;
    esac
  done <<<"$changed"

  for svc in "${!build_pick[@]}"; do
    LW_BUILD+=("$svc")
  done
  for svc in "${!recreate_pick[@]}"; do
    LW_RECREATE+=("$svc")
  done

  if [ "${#LW_BUILD[@]}" -gt 0 ]; then
    mapfile -t LW_BUILD < <(printf '%s\n' "${LW_BUILD[@]}" | sort -u)
  fi
  if [ "${#LW_RECREATE[@]}" -gt 0 ]; then
    mapfile -t LW_RECREATE < <(printf '%s\n' "${LW_RECREATE[@]}" | sort -u)
  fi
}

laptopworker_run_migrate() {
  local root="$1"
  echo "==> running migrate"
  laptopworker_compose "$root" run --rm migrate
}

# Backward-compatible helper (stdout: one service per line).
laptopworker_plan_build_services() {
  local root="$1"
  local old_sha="$2"
  local new_sha="$3"
  laptopworker_plan_update "$root" "$old_sha" "$new_sha"
  printf '%s\n' "${LW_BUILD[@]}"
}

laptopworker_compose_up() {
  local root="$1"
  shift
  laptopworker_compose "$root" up -d --remove-orphans "$@"
}
