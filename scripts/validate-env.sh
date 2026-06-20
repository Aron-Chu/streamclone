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

if [ "$PROFILE" = "clipper" ]; then
  warn "Profile clipper is deprecated — compose uses core only; install ReplayForge separately" "See docs/agents-streamclone-and-replayforge.md"
fi

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
non_loopback_origin=false
public_origin="$(env_read_value "$ENV_FILE" PUBLIC_ORIGIN || true)"
frontend_origin="$(env_read_value "$ENV_FILE" FRONTEND_ORIGIN || true)"
if [ "$loopback_install" != true ] && { [ -n "$public_origin" ] || [ -n "$frontend_origin" ]; }; then
  non_loopback_origin=true
fi
if [ "$dev_import" = "true" ] && [ "$non_loopback_origin" = true ]; then
  fail "TWITCH_DEV_TOKEN_IMPORT_ENABLED=true with a non-loopback PUBLIC_ORIGIN/FRONTEND_ORIGIN" "Set TWITCH_DEV_TOKEN_IMPORT_ENABLED=false before using a tunnel or public domain"
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

if [ "$non_loopback_origin" = true ]; then
  setup_token="$(env_read_value "$ENV_FILE" SETUP_CONTROL_TOKEN || true)"
  clipper_browser_token="$(env_read_value "$ENV_FILE" VITE_CLIPPER_TOKEN || true)"
  if [ -n "$setup_token" ]; then
    warn "SETUP_CONTROL_TOKEN is exposed to browsers through /config.js on a non-loopback origin" "Unset SETUP_CONTROL_TOKEN or restrict tunnel access; setup-control mutations are intended for trusted localhost only"
  fi
  if [ -n "$clipper_browser_token" ]; then
    warn "VITE_CLIPPER_TOKEN is exposed to browsers through /config.js on a non-loopback origin" "Unset VITE_CLIPPER_TOKEN unless every visitor is trusted to call clipper mutation APIs"
  fi
  s3_access="$(env_read_value "$ENV_FILE" S3_ACCESS_KEY || true)"
  s3_secret="$(env_read_value "$ENV_FILE" S3_SECRET_KEY || true)"
  if [ "$s3_access" = "minioadmin" ] || [ "$s3_secret" = "minioadmin" ]; then
    warn "MinIO root credentials are still the local defaults on a non-loopback origin" "Rotate S3_ACCESS_KEY/S3_SECRET_KEY before treating this install as production-ready"
  fi
fi

oauth_id="$(env_read_value "$ENV_FILE" TWITCH_OAUTH_CLIENT_ID || true)"
oauth_secret="$(env_read_value "$ENV_FILE" TWITCH_OAUTH_CLIENT_SECRET || true)"
if [ -z "$oauth_id" ] || [ -z "$oauth_secret" ]; then
  if [ "$dev_import" = "true" ]; then
    warn "Sign in (optional) requires TWITCH_OAUTH_CLIENT_ID/SECRET" "Run twitch configure then make twitch-sync, or copy deploy/env/oauth-bundle.env.example to deploy/env/oauth-bundle.env and re-run setup"
  else
    case "$PROFILE" in
      scraper|full)
        warn "TWITCH_OAUTH_CLIENT_ID/SECRET missing from $ENV_FILE" "Streamclone can still start; Helix VOD lookup/token refresh may be limited until you run twitch configure then make twitch-sync"
        ;;
      *)
        warn "TWITCH_OAUTH_CLIENT_ID/SECRET missing from $ENV_FILE" "Optional Helix enrichment and token refresh are limited until you run: make twitch-sync"
        ;;
    esac
  fi
fi

pulse_wire="$(env_read_value "$ENV_FILE" PULSE_WIRE_ENABLED || true)"
reddit_ok="$(env_read_value "$ENV_FILE" REDDIT_COMMERCIAL_OK || true)"
if [ "$pulse_wire" = "true" ] && [ "$reddit_ok" != "true" ]; then
  warn "PULSE_WIRE_ENABLED=true but REDDIT_COMMERCIAL_OK is not true" "Reddit ingest stays disabled until you accept commercial API terms (set REDDIT_COMMERCIAL_OK=true in .env.local)"
fi

streamerbans_enabled="$(env_read_value "$ENV_FILE" STREAMERBANS_INGEST_ENABLED || true)"
x_unofficial_ok="$(env_read_value "$ENV_FILE" X_UNOFFICIAL_OK || true)"
x_auth_token="$(env_read_value "$ENV_FILE" X_AUTH_TOKEN || true)"
emusks_x_auth_token="$(env_read_value "$ENV_FILE" EMUSKS_X_AUTH_TOKEN || true)"
has_x_token=false
if [ -n "$x_auth_token" ] || [ -n "$emusks_x_auth_token" ]; then
  has_x_token=true
fi
if [ "$x_unofficial_ok" = "true" ] && [ "$has_x_token" != true ]; then
  warn "X_UNOFFICIAL_OK=true but no X_AUTH_TOKEN or EMUSKS_X_AUTH_TOKEN is set" "StreamerBans tier 2 is credential-gated; leave tier 2 unset for HTML fallback, or keep the token in .env.local"
fi
if [ "$has_x_token" = true ] && [ "$x_unofficial_ok" != "true" ]; then
  warn "X_AUTH_TOKEN/EMUSKS_X_AUTH_TOKEN is set but X_UNOFFICIAL_OK is not true" "Set X_UNOFFICIAL_OK=true only if you accept the unofficial emusks/X path; otherwise remove the token"
fi
if { [ "$x_unofficial_ok" = "true" ] || [ "$has_x_token" = true ]; } && [ "$streamerbans_enabled" != "true" ]; then
  warn "StreamerBans tier-2 variables are set but STREAMERBANS_INGEST_ENABLED is not true" "Tier 2 only augments StreamerBans ingest; set STREAMERBANS_INGEST_ENABLED=true or remove the tier-2 variables"
fi

case "$PROFILE" in
  scraper|full)
    require_nonempty SCRAPER_API_URL "Profile scraper sets SCRAPER_API_URL=http://scraper:8000/v2/scrape"
    require_not_placeholder SCRAPER_API_KEY local-dev-key "Run make setup or scripts/validate-env.sh --fix"
    scraper_use_images="$(env_read_value "$ENV_FILE" SCRAPER_USE_IMAGES || true)"
    if [ "$scraper_use_images" != "1" ]; then
      sibling="$(env_scraper_sibling_path)"
      if [ ! -d "$sibling/.git" ] && [ ! -f "$sibling/Dockerfile" ]; then
        fail "streamclone-scraper sibling missing at $sibling" "git clone https://github.com/Aron-Chu/streamclone-scraper.git $sibling or set SCRAPER_USE_IMAGES=1"
      fi
    fi
    ;;
esac

clipper_token="$(env_read_value "$ENV_FILE" CLIPPER_TWITCH_USER_ACCESS_TOKEN || true)"
if [ -n "$clipper_token" ]; then
  if ! curl -fsS --max-time 3 http://127.0.0.1:8095/healthz >/dev/null 2>&1; then
    warn "CLIPPER_TWITCH_USER_ACCESS_TOKEN is set but ReplayForge is not reachable at http://127.0.0.1:8095/healthz" "Install and start ReplayForge separately — see docs/agents-streamclone-and-replayforge.md"
  fi
fi

echo ""
if [ "$errors" -gt 0 ]; then
  echo "validate-env: FAILED — $errors error(s), $warnings warning(s)"
  exit 1
fi
echo "validate-env: OK — $warnings warning(s)"
exit 0
