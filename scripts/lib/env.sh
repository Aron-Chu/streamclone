#!/usr/bin/env bash
# Shared env merge + secret generation for bootstrap/setup/validate.
set -euo pipefail

ENV_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_REPO_ROOT="$(cd "$ENV_LIB_DIR/../.." && pwd)"

env_repo_root() {
  printf '%s\n' "$ENV_REPO_ROOT"
}

env_random_hex() {
  local nbytes="${1:-24}"
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex "$nbytes"
  else
    head -c "$nbytes" /dev/urandom | od -An -tx1 | tr -d ' \n'
  fi
}

env_profile_fragment() {
  local profile="$1"
  case "$profile" in
    core) printf '%s\n' "$ENV_REPO_ROOT/deploy/env/profile-core.env" ;;
    scraper) printf '%s\n' "$ENV_REPO_ROOT/deploy/env/profile-scraper.env" ;;
    clipper) printf '%s\n' "$ENV_REPO_ROOT/deploy/env/profile-clipper.env" ;;
    full) printf '%s\n' "$ENV_REPO_ROOT/deploy/env/profile-full.env" ;;
    *)
      echo "env_profile_fragment: unknown profile '$profile' (use core|scraper|clipper|full)" >&2
      return 1
      ;;
  esac
}

env_compose_profiles() {
  local profile="$1"
  case "$profile" in
    core) printf '%s' "" ;;
    scraper) printf '%s' "scraper" ;;
    clipper) printf '%s' "clipper" ;;
    full) printf '%s' "scraper clipper" ;;
    *)
      echo "env_compose_profiles: unknown profile '$profile'" >&2
      return 1
      ;;
  esac
}

# Parse KEY=VALUE lines into associative array name passed by reference (bash 4+).
env_load_into() {
  local -n _out="$1"
  local file="$2"
  _out=()
  [ -f "$file" ] || return 0
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      ''|'#'*) continue ;;
      *=*)
        local key="${line%%=*}"
        local value="${line#*=}"
        _out["$key"]="$value"
        ;;
    esac
  done <"$file"
}

env_write_from_map() {
  local outfile="$1"
  shift
  local -n _keys_ref="$1"
  local -n _map_ref="$2"
  {
    for key in "${_keys_ref[@]}"; do
      printf '%s=%s\n' "$key" "${_map_ref[$key]}"
    done
  } >"$outfile"
}

env_merge_files() {
  local outfile="$1"
  shift
  declare -A merged=()
  local -a key_order=()

  _env_merge_record() {
    local key="$1"
    local value="$2"
    if [[ -z "${merged[$key]+x}" ]]; then
      key_order+=("$key")
    fi
    merged["$key"]="$value"
  }

  for src in "$@"; do
    [ -f "$src" ] || continue
    while IFS= read -r line || [ -n "$line" ]; do
      case "$line" in
        ''|'#'*) continue ;;
        *=*)
          _env_merge_record "${line%%=*}" "${line#*=}"
          ;;
      esac
    done <"$src"
  done

  env_write_from_map "$outfile" key_order merged
}

env_set_key() {
  local file="$1"
  local key="$2"
  local value="$3"
  local tmp
  tmp="$(mktemp)"
  if [ -f "$file" ]; then
    local found=false
    while IFS= read -r line || [ -n "$line" ]; do
      if [[ "$line" == "$key="* ]]; then
        printf '%s=%s\n' "$key" "$value"
        found=true
      else
        printf '%s\n' "$line"
      fi
    done <"$file" >"$tmp"
    if [ "$found" = false ]; then
      printf '%s=%s\n' "$key" "$value" >>"$tmp"
    fi
  else
    printf '%s=%s\n' "$key" "$value" >"$tmp"
  fi
  mv "$tmp" "$file"
}

env_placeholder_value() {
  local key="$1"
  local value="$2"
  case "$key" in
    CURATOR_API_TOKEN)
      [ "$value" = "change-me" ] || [ -z "$value" ]
      ;;
    AUTH_COOKIE_SECRET)
      [ "$value" = "dev-insecure-cookie-secret" ] || [ -z "$value" ]
      ;;
    SCRAPER_API_KEY)
      [ "$value" = "local-dev-key" ] || [ -z "$value" ]
      ;;
    CLIPPER_WEBHOOK_TOKEN|VITE_CLIPPER_TOKEN)
      [ -z "$value" ]
      ;;
    *)
      return 1
      ;;
  esac
}

env_generate_secrets() {
  local file="$1"
  local curator auth scraper clipper

  declare -A current=()
  env_load_into current "$file"

  curator="${current[CURATOR_API_TOKEN]:-}"
  if env_placeholder_value CURATOR_API_TOKEN "$curator"; then
    curator="$(env_random_hex 24)"
    env_set_key "$file" CURATOR_API_TOKEN "$curator"
  fi

  auth="${current[AUTH_COOKIE_SECRET]:-}"
  if env_placeholder_value AUTH_COOKIE_SECRET "$auth"; then
    auth="$(env_random_hex 32)"
    env_set_key "$file" AUTH_COOKIE_SECRET "$auth"
  fi

  scraper="${current[SCRAPER_API_KEY]:-}"
  if env_placeholder_value SCRAPER_API_KEY "$scraper"; then
    scraper="$(env_random_hex 16)"
    env_set_key "$file" SCRAPER_API_KEY "$scraper"
  fi

  clipper="${current[CLIPPER_WEBHOOK_TOKEN]:-}"
  if env_placeholder_value CLIPPER_WEBHOOK_TOKEN "$clipper"; then
    clipper="$(env_random_hex 24)"
    env_set_key "$file" CLIPPER_WEBHOOK_TOKEN "$clipper"
    env_set_key "$file" VITE_CLIPPER_TOKEN "$clipper"
  elif [ -z "${current[VITE_CLIPPER_TOKEN]:-}" ] && [ -n "$clipper" ]; then
    env_set_key "$file" VITE_CLIPPER_TOKEN "$clipper"
  fi
}

env_release_version_tag() {
  local version_file="$ENV_REPO_ROOT/VERSION"
  [ -f "$version_file" ] || return 1
  local tag
  tag="$(tr -d '[:space:]' <"$version_file")"
  [ -n "$tag" ] || return 1
  printf '%s' "$tag"
}

env_apply_release_image_tag() {
  local outfile="$1"
  local current
  current="$(env_read_value "$outfile" IMAGE_TAG 2>/dev/null || true)"
  [ -n "$current" ] && return 0
  local tag
  tag="$(env_release_version_tag || true)"
  [ -n "$tag" ] || return 0
  env_set_key "$outfile" IMAGE_TAG "$tag"
  env_set_key "$outfile" STREAMCLONE_USE_IMAGES 1
}

env_synthesize() {
  local profile="${1:-core}"
  local outfile="${2:-$ENV_REPO_ROOT/.env}"
  local fragment
  fragment="$(env_profile_fragment "$profile")"

  local -a sources=("$ENV_REPO_ROOT/.env.dev" "$fragment")
  if [ -f "$ENV_REPO_ROOT/deploy/env/release-bundle.env" ]; then
    sources+=("$ENV_REPO_ROOT/deploy/env/release-bundle.env")
  fi
  if [ -f "$ENV_REPO_ROOT/.env.local" ]; then
    sources+=("$ENV_REPO_ROOT/.env.local")
  fi

  env_merge_files "$outfile" "${sources[@]}"
  env_set_key "$outfile" STREAMCLONE_PROFILE "$profile"
  env_generate_secrets "$outfile"
  env_apply_release_image_tag "$outfile"
}

env_read_value() {
  local file="$1"
  local key="$2"
  [ -f "$file" ] || return 1
  while IFS= read -r line || [ -n "$line" ]; do
    if [[ "$line" == "$key="* ]]; then
      printf '%s' "${line#*=}"
      return 0
    fi
  done <"$file"
  return 1
}

env_scraper_sibling_path() {
  printf '%s\n' "$(cd "$ENV_REPO_ROOT/.." && pwd)/streamclone-scraper"
}

env_preflight_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    echo "Docker is required. Install Docker Desktop and ensure 'docker' is on PATH." >&2
    return 1
  fi
  if ! docker compose version >/dev/null 2>&1; then
    echo "docker compose is required. Update Docker Desktop." >&2
    return 1
  fi
}

env_twitch_cli_config_path() {
  if [ -n "${APPDATA:-}" ] && [ -f "$APPDATA/twitch-cli/.twitch-cli.env" ]; then
    printf '%s\n' "$APPDATA/twitch-cli/.twitch-cli.env"
  elif [ -f "$HOME/.config/twitch-cli/.twitch-cli.env" ]; then
    printf '%s\n' "$HOME/.config/twitch-cli/.twitch-cli.env"
  elif [ -f "$HOME/Library/Application Support/twitch-cli/.twitch-cli.env" ]; then
    printf '%s\n' "$HOME/Library/Application Support/twitch-cli/.twitch-cli.env"
  else
    return 1
  fi
}

env_sync_twitch_cli() {
  local env_file="${1:-$ENV_REPO_ROOT/.env}"
  local cli_config
  cli_config="$(env_twitch_cli_config_path)" || {
    echo "Twitch CLI config not found. Run: twitch configure" >&2
    return 1
  }
  local client_id client_secret
  client_id="$(env_read_value "$cli_config" CLIENTID || true)"
  client_secret="$(env_read_value "$cli_config" CLIENTSECRET || true)"
  if [ -z "$client_id" ] || [ -z "$client_secret" ]; then
    echo "Twitch CLI config missing CLIENTID or CLIENTSECRET." >&2
    return 1
  fi
  env_set_key "$env_file" TWITCH_OAUTH_CLIENT_ID "$client_id"
  env_set_key "$env_file" TWITCH_OAUTH_CLIENT_SECRET "$client_secret"
}
