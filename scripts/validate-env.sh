#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=lib/env.sh
source "$ROOT/scripts/lib/env.sh"

PROFILE="core"
ENV_FILE="$ROOT/.env"
FIX=false

usage() {
  cat <<'EOF'
Usage: scripts/validate-env.sh [options]

  --profile core|scraper|clipper|full   Profile to validate (default: core)
  --env-file PATH                       Env file (default: .env)
  --fix                                 Regenerate placeholder secrets in place
  -h, --help                            Show help
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --profile) PROFILE="$2"; shift 2 ;;
    --env-file) ENV_FILE="$2"; shift 2 ;;
    --fix) FIX=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

case "$PROFILE" in
  core|scraper|clipper|full) ;;
  *) echo "Invalid profile: $PROFILE" >&2; exit 1 ;;
esac

errors=0
warnings=0

fail() {
  echo "ERROR: $1" >&2
  echo "  fix: $2" >&2
  errors=$((errors + 1))
}

warn() {
  echo "WARN: $1" >&2
  echo "  hint: $2" >&2
  warnings=$((warnings + 1))
}

if [ ! -f "$ENV_FILE" ]; then
  fail "Missing env file at $ENV_FILE" "make setup or scripts/setup.sh --profile $PROFILE --non-interactive --no-up"
  echo ""
  echo "validate-env: $errors error(s), $warnings warning(s)"
  exit 1
fi

if [ "$FIX" = true ]; then
  env_generate_secrets "$ENV_FILE"
  echo "Regenerated placeholder secrets in $ENV_FILE"
fi

require_nonempty() {
  local key="$1"
  local fix_hint="$2"
  local val
  val="$(env_read_value "$ENV_FILE" "$key" || true)"
  if [ -z "$val" ]; then
    fail "$key is empty" "$fix_hint"
  fi
}

require_not_placeholder() {
  local key="$1"
  local placeholder="$2"
  local fix_hint="$3"
  local val
  val="$(env_read_value "$ENV_FILE" "$key" || true)"
  if [ -z "$val" ] || [ "$val" = "$placeholder" ]; then
    fail "$key is missing or still '$placeholder'" "$fix_hint"
  fi
}

echo "validate-env: profile=$PROFILE file=$ENV_FILE"

require_nonempty DATABASE_URL "Run make setup to synthesize .env from .env.dev"
require_nonempty REDIS_URL "Run make setup to synthesize .env from .env.dev"
require_not_placeholder CURATOR_API_TOKEN change-me "Run make setup or scripts/validate-env.sh --fix"
require_nonempty PUBLIC_ORIGIN "Set PUBLIC_ORIGIN=http://localhost:8090 for local dev"
require_nonempty FRONTEND_ORIGIN "Set FRONTEND_ORIGIN=http://localhost:8090"

dev_import="$(env_read_value "$ENV_FILE" TWITCH_DEV_TOKEN_IMPORT_ENABLED || true)"
use_images="$(env_read_value "$ENV_FILE" STREAMCLONE_USE_IMAGES || true)"
loopback_install=false
if env_loopback_public_origin "$ENV_FILE" 2>/dev/null; then
  loopback_install=true
fi
if [ "$use_images" = "1" ]; then
  if [ "$dev_import" = "true" ] && [ "$loopback_install" != true ]; then
    warn "TWITCH_DEV_TOKEN_IMPORT_ENABLED=true on a non-loopback release install" "Dev-only token import should stay false outside localhost (docs/security.md)"
  elif [ "$dev_import" != "true" ] && [ "$loopback_install" = true ]; then
    warn "TWITCH_DEV_TOKEN_IMPORT_ENABLED is not true on loopback release install" "Run scripts/reload-env-if-stale.ps1 or restart Streamclone to enable Sign in (optional)"
  fi
elif [ "$dev_import" != "true" ]; then
  warn "TWITCH_DEV_TOKEN_IMPORT_ENABLED is not true" "Run make setup (.env.dev sets this for in-app local token import)"
fi

oauth_id="$(env_read_value "$ENV_FILE" TWITCH_OAUTH_CLIENT_ID || true)"
oauth_secret="$(env_read_value "$ENV_FILE" TWITCH_OAUTH_CLIENT_SECRET || true)"
if [ -z "$oauth_id" ] || [ -z "$oauth_secret" ]; then
  case "$PROFILE" in
    scraper|full)
      fail "TWITCH_OAUTH_CLIENT_ID/SECRET missing from $ENV_FILE" "Run twitch configure then make twitch-sync (analytics emote seeding and Helix need these)"
      ;;
    *)
      warn "TWITCH_OAUTH_CLIENT_ID/SECRET missing from $ENV_FILE" "Analytics emote charts and Helix will fail until you run: make twitch-sync"
      ;;
  esac
fi

case "$PROFILE" in
  scraper|full)
    require_nonempty SCRAPER_API_URL "Profile scraper sets SCRAPER_API_URL=http://scraper:8000/v2/scrape"
    require_not_placeholder SCRAPER_API_KEY local-dev-key "Run make setup or scripts/validate-env.sh --fix"
    sibling="$(env_scraper_sibling_path)"
    if [ ! -d "$sibling/.git" ] && [ ! -f "$sibling/Dockerfile" ]; then
      fail "streamclone-scraper sibling missing at $sibling" "git clone https://github.com/Aron-Chu/streamclone-scraper.git $sibling"
    fi
    ;;
esac

case "$PROFILE" in
  clipper|full)
    webhook="$(env_read_value "$ENV_FILE" CLIPPER_WEBHOOK_TOKEN || true)"
    vite="$(env_read_value "$ENV_FILE" VITE_CLIPPER_TOKEN || true)"
    if [ -z "$webhook" ]; then
      fail "CLIPPER_WEBHOOK_TOKEN is empty" "Run make setup or scripts/validate-env.sh --fix"
    fi
    if [ -z "$vite" ] || [ "$vite" != "$webhook" ]; then
      warn "VITE_CLIPPER_TOKEN should match CLIPPER_WEBHOOK_TOKEN" "Run make setup or scripts/validate-env.sh --fix"
    fi
    token="$(env_read_value "$ENV_FILE" CLIPPER_TWITCH_USER_ACCESS_TOKEN || true)"
    if [ -z "$token" ]; then
      warn "CLIPPER_TWITCH_USER_ACCESS_TOKEN is empty" "Click Sign in (optional) at http://localhost:8090, or run make twitch-local-auth with Twitch CLI"
    fi
    client="$(env_read_value "$ENV_FILE" CLIPPER_TWITCH_CLIENT_ID || true)"
    oauth="$(env_read_value "$ENV_FILE" TWITCH_OAUTH_CLIENT_ID || true)"
    if [ -z "$client" ] && [ -z "$oauth" ]; then
      warn "No Twitch OAuth client id for clipper" "Run twitch configure then make twitch-sync"
    fi
    ;;
esac

echo ""
if [ "$errors" -gt 0 ]; then
  echo "validate-env: FAILED — $errors error(s), $warnings warning(s)"
  exit 1
fi
echo "validate-env: OK — $warnings warning(s)"
exit 0
