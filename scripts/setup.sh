#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=lib/env.sh
source "$ROOT/scripts/lib/env.sh"

cd "$ROOT"

PROFILE="core"
NON_INTERACTIVE=false
USE_IMAGES=false
SKIP_UP=false
SKIP_SMOKE=false
SKIP_TWITCH=false
SKIP_SCRAPER_CLONE=false

usage() {
  cat <<'EOF'
Usage: scripts/setup.sh [options]

  --profile core|scraper|clipper|full   Stack profile (default: core)
  --non-interactive                     No prompts; use defaults
  --use-images                          Pull pre-built GHCR images (release overlay)
  --no-up                               Synthesize .env only; do not start compose
  --no-smoke                            Skip post-start health checks
  --skip-twitch                         Skip Twitch CLI OAuth sync
  --skip-scraper-clone                  Do not offer cloning streamclone-scraper
  -h, --help                            Show this help
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --profile) PROFILE="$2"; shift 2 ;;
    --non-interactive) NON_INTERACTIVE=true; shift ;;
    --use-images) USE_IMAGES=true; shift ;;
    --no-up) SKIP_UP=true; shift ;;
    --no-smoke) SKIP_SMOKE=true; shift ;;
    --skip-twitch) SKIP_TWITCH=true; shift ;;
    --skip-scraper-clone) SKIP_SCRAPER_CLONE=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 1 ;;
  esac
done

case "$PROFILE" in
  core|scraper|clipper|full) ;;
  *)
    echo "Invalid profile: $PROFILE" >&2
    exit 1
    ;;
esac

if [ "${SETUP_MODE:-}" = "release" ]; then
  USE_IMAGES=true
fi

echo "Streamclone setup"
echo "─────────────────"

if [ "$NON_INTERACTIVE" = false ]; then
  cat <<'MENU'

[1] Core only          — watch + chat + emotes (no Twitch login required)
[2] + Analytics charts — uses scraper image or streamclone-scraper sibling repo
[3] Full stack         — core + Analytics scraper

MENU
  read -r -p "Choose profile [1-3, default 1]: " choice
  case "${choice:-1}" in
    1|"") PROFILE=core ;;
    2) PROFILE=scraper ;;
    3) PROFILE=full ;;
    *) echo "Invalid choice; using core."; PROFILE=core ;;
  esac
fi

echo "Profile: $PROFILE"

if [ "$PROFILE" = "clipper" ] || [ "$PROFILE" = "full" ]; then
  if command -v powershell.exe >/dev/null 2>&1; then
    powershell.exe -ExecutionPolicy Bypass -File "$ROOT/scripts/ensure-replayforge-hint.ps1" || true
  elif command -v pwsh >/dev/null 2>&1; then
    pwsh -ExecutionPolicy Bypass -File "$ROOT/scripts/ensure-replayforge-hint.ps1" || true
  fi
fi

env_preflight_docker
echo "Docker: ok"

env_synthesize "$PROFILE" .env
printf '%s' "$PROFILE" >.streamclone-profile
var_count="$(grep -c '^[A-Z]' .env || true)"
echo ".env: synthesized ($var_count keys, secrets generated)"

oauth_id="$(env_read_value .env TWITCH_OAUTH_CLIENT_ID || true)"
oauth_secret="$(env_read_value .env TWITCH_OAUTH_CLIENT_SECRET || true)"
if [ -z "$oauth_id" ] || [ -z "$oauth_secret" ]; then
  echo "Note: Twitch OAuth app creds not set — optional Sign in and Helix features need them; Reddit-only Pulse Wire still works."
fi

if command -v powershell.exe >/dev/null 2>&1; then
  powershell.exe -ExecutionPolicy Bypass -File "$ROOT/scripts/ensure-oauth-env.ps1" -EnvFile "$ROOT/.env" || true
elif command -v pwsh >/dev/null 2>&1; then
  pwsh -ExecutionPolicy Bypass -File "$ROOT/scripts/ensure-oauth-env.ps1" -EnvFile "$ROOT/.env" || true
fi

# Twitch CLI (optional)
if [ "$SKIP_TWITCH" = false ]; then
  if command -v twitch >/dev/null 2>&1; then
    echo "Twitch CLI: found"
    if env_twitch_cli_config_path >/dev/null 2>&1; then
      sync=false
      if [ "$NON_INTERACTIVE" = true ]; then
        sync=true
      else
        read -r -p "Sync OAuth app creds from twitch CLI to .env? [Y/n]: " ans
        case "${ans:-Y}" in
          y|Y|yes|YES) sync=true ;;
        esac
      fi
      if [ "$sync" = true ]; then
        env_sync_twitch_cli .env && echo "  synced TWITCH_OAUTH_CLIENT_ID/SECRET"
      fi
    else
      echo "  twitch configure not run yet — skip or run: twitch configure"
    fi
  else
    echo "Twitch CLI: not found — https://github.com/twitchdev/twitch-cli"
    echo "  Sign in (optional) still needs TWITCH_OAUTH_CLIENT_ID/SECRET in .env."
    echo "  Copy deploy/env/oauth-bundle.env.example to oauth-bundle.env, or install twitch-cli."
  fi
fi

dev_import="$(env_read_value .env TWITCH_DEV_TOKEN_IMPORT_ENABLED || true)"
oauth_id="$(env_read_value .env TWITCH_OAUTH_CLIENT_ID || true)"
oauth_secret="$(env_read_value .env TWITCH_OAUTH_CLIENT_SECRET || true)"
if [ "$dev_import" = "true" ] && { [ -z "$oauth_id" ] || [ -z "$oauth_secret" ]; }; then
  echo "Sign in (optional) will fail until TWITCH_OAUTH_CLIENT_ID/SECRET are set (twitch configure + make twitch-sync, or oauth-bundle.env)."
fi

# Scraper sibling
needs_scraper=false
case "$PROFILE" in
  scraper|full) needs_scraper=true ;;
esac
scraper_use_images="$(env_read_value .env SCRAPER_USE_IMAGES || true)"

if [ "$needs_scraper" = true ] && [ "$scraper_use_images" = "1" ]; then
  echo "Scraper: GHCR image (SCRAPER_USE_IMAGES=1)"
elif [ "$needs_scraper" = true ] && [ "$SKIP_SCRAPER_CLONE" = false ]; then
  sibling="$(env_scraper_sibling_path)"
  if [ -d "$sibling/.git" ] || [ -f "$sibling/Dockerfile" ]; then
    echo "Scraper repo: ok ($sibling)"
  else
    echo "Scraper repo: missing at $sibling"
    clone=false
    if [ "$NON_INTERACTIVE" = true ]; then
      clone=false
    else
      read -r -p "Clone https://github.com/Aron-Chu/streamclone-scraper? [Y/n]: " ans
      case "${ans:-Y}" in
        y|Y|yes|YES) clone=true ;;
      esac
    fi
    if [ "$clone" = true ]; then
      git clone https://github.com/Aron-Chu/streamclone-scraper.git "$sibling"
      echo "  cloned to $sibling"
    else
      echo "  scraper profile disabled until sibling repo exists."
    fi
  fi

  if [ "$NON_INTERACTIVE" = false ]; then
    read -r -p "Set PROXY_* vars in .env.local for TwitchTracker egress? [y/N]: " proxy_ans
    case "$proxy_ans" in
      y|Y|yes|YES)
        read -r -p "PROXY_SERVER: " proxy_server
        if [ -n "$proxy_server" ]; then
          {
            echo "PROXY_SERVER=$proxy_server"
            read -r -p "PROXY_USERNAME (optional): " proxy_user
            [ -n "$proxy_user" ] && echo "PROXY_USERNAME=$proxy_user"
            read -r -s -p "PROXY_PASSWORD (optional): " proxy_pass
            echo ""
            [ -n "$proxy_pass" ] && echo "PROXY_PASSWORD=$proxy_pass"
          } >>.env.local
          env_synthesize "$PROFILE" .env
          echo "  wrote proxy settings to .env.local and re-merged .env"
        fi
        ;;
    esac
  fi
fi

compose_args=(--env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml)
if [ "$USE_IMAGES" = true ]; then
  compose_args+=(-f deploy/docker-compose.release.yml)
fi

read -r -a profiles <<<"$(env_streamclone_compose_profiles "$PROFILE" "$ROOT/.env")"
for p in "${profiles[@]}"; do
  [ -n "$p" ] && compose_args+=(--profile "$p")
done

if [ "$SKIP_UP" = false ]; then
  up_flags=(-d --remove-orphans)
  if [ "$USE_IMAGES" = true ]; then
    echo "Pulling Docker images..."
    docker "${compose_args[@]}" pull
    up_flags+=(--pull missing)
  else
    up_flags+=(--build)
  fi
  echo "Starting stack (profile: $PROFILE)..."
  docker compose "${compose_args[@]}" up "${up_flags[@]}"
  if grep -q '^TWITCH_OAUTH_CLIENT_ID=' .env 2>/dev/null; then
    echo "Checking container OAuth env matches .env..."
    if command -v powershell.exe >/dev/null 2>&1; then
      powershell.exe -ExecutionPolicy Bypass -File "$ROOT/scripts/reload-env-if-stale.ps1" -EnvFile "$ROOT/.env" || true
    elif command -v pwsh >/dev/null 2>&1; then
      pwsh -ExecutionPolicy Bypass -File "$ROOT/scripts/reload-env-if-stale.ps1" -EnvFile "$ROOT/.env" || true
    fi
  fi
  echo ""
  echo "Streamclone: http://localhost:8090"
fi

if [ "$SKIP_SMOKE" = false ] && [ "$SKIP_UP" = false ]; then
  echo "Waiting for tiered readiness..."
  bash "$ROOT/scripts/lib/wait-stack.sh"
  echo "Running smoke checks (readiness already verified)..."
  bash "$ROOT/scripts/smoke-core.sh" --skip-readiness

  if [ "$needs_scraper" = true ] && { [ "$scraper_use_images" = "1" ] || [ -d "$(env_scraper_sibling_path)" ]; }; then
    if [ -z "${SCRAPER_SKIP_PREFLIGHT:-}" ]; then
      echo "Running scraper preflight (Camoufox TwitchTracker probe)..."
      if ! bash "$ROOT/scripts/scraper-preflight.sh"; then
        echo "  scraper preflight failed — see hints above or run: make scraper-warm" >&2
        exit 1
      fi
    else
      echo "Checking scraper health (SCRAPER_SKIP_PREFLIGHT=1)..."
      for i in $(seq 1 30); do
        if curl -fsS http://localhost:8000/health >/dev/null 2>&1; then
          echo "  scraper ok"
          break
        fi
        [ "$i" -eq 30 ] && echo "  scraper health check timed out (profile may still be starting)" >&2
        sleep 2
      done
    fi
  fi
fi

echo "Setup complete."
