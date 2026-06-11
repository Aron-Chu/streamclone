#!/usr/bin/env bash
# One-command start for non-developers: check deps, setup if needed, start stack, open browser.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=lib/env.sh
source "$ROOT/scripts/lib/env.sh"

PROFILE=""
USE_IMAGES=false
USE_IMAGES_SET=false
NO_BROWSER=false
SKIP_SETUP=false

usage() {
  cat <<'EOF'
Usage: scripts/start-streamclone.sh [options]

  --profile core|scraper|clipper|full
  --use-images          Pull GHCR images instead of building
  --no-browser          Do not open http://localhost:8090
  --skip-setup          Require existing .env
EOF
}

streamclone_use_images_default() {
  [ -f "$ROOT/VERSION" ] && return 0
  [ "${STREAMCLONE_USE_IMAGES:-}" = "1" ] && return 0
  if [ -f "$ROOT/.env" ]; then
    local val
    val="$(env_read_value "$ROOT/.env" STREAMCLONE_USE_IMAGES || true)"
    [ "$val" = "1" ] && return 0
  fi
  return 1
}

while [ $# -gt 0 ]; do
  case "$1" in
    --profile) PROFILE="$2"; shift 2 ;;
    --use-images) USE_IMAGES=true; USE_IMAGES_SET=true; shift ;;
    --no-browser) NO_BROWSER=true; shift ;;
    --skip-setup) SKIP_SETUP=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

cd "$ROOT"

bash "$ROOT/scripts/preflight-deps.sh" --install-hints

if [ -z "$PROFILE" ] && [ -f "$ROOT/.streamclone-profile" ]; then
  PROFILE="$(tr -d ' \n\r' <"$ROOT/.streamclone-profile")"
fi
PROFILE="${PROFILE:-core}"

if [ "$USE_IMAGES_SET" = false ] && streamclone_use_images_default; then
  USE_IMAGES=true
fi

if [ "$SKIP_SETUP" = false ] && [ ! -f "$ROOT/.env" ]; then
  echo ""
  echo "First run — running setup (profile: $PROFILE)..."
  setup_args=(--profile "$PROFILE" --non-interactive)
  [ "$USE_IMAGES" = true ] && setup_args+=(--use-images)
  bash "$ROOT/scripts/setup.sh" "${setup_args[@]}"
else
  if [ ! -f "$ROOT/.env" ]; then
    echo "Missing .env — run: scripts/setup.sh" >&2
    exit 1
  fi
  echo "Starting Streamclone (profile: $PROFILE)..."
  if [ "$USE_IMAGES" = true ]; then
    echo "Using pre-built GHCR images (release bundle or STREAMCLONE_USE_IMAGES=1)."
  fi
  compose_args=(--env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml)
  [ "$USE_IMAGES" = true ] && compose_args+=(-f deploy/docker-compose.release.yml)
  read -r -a profiles <<<"$(env_compose_profiles "$PROFILE")"
  for p in "${profiles[@]}"; do
    [ -n "$p" ] && compose_args+=(--profile "$p")
  done
  up_flags=(-d --remove-orphans)
  if [ "$USE_IMAGES" = true ]; then
    up_flags+=(--pull missing)
  else
    up_flags+=(--build)
  fi
  docker compose "${compose_args[@]}" up "${up_flags[@]}"

  if command -v powershell.exe >/dev/null 2>&1; then
    powershell.exe -ExecutionPolicy Bypass -File "$ROOT/scripts/reload-env-if-stale.ps1" -EnvFile "$ROOT/.env" || true
  elif command -v pwsh >/dev/null 2>&1; then
    pwsh -ExecutionPolicy Bypass -File "$ROOT/scripts/reload-env-if-stale.ps1" -EnvFile "$ROOT/.env" || true
  fi
  case "$PROFILE" in
    clipper|full)
      if command -v powershell.exe >/dev/null 2>&1; then
        powershell.exe -ExecutionPolicy Bypass -File "$ROOT/scripts/ensure-clipper-auth.ps1" -EnvFile "$ROOT/.env" || true
      elif command -v pwsh >/dev/null 2>&1; then
        pwsh -ExecutionPolicy Bypass -File "$ROOT/scripts/ensure-clipper-auth.ps1" -EnvFile "$ROOT/.env" || true
      fi
      ;;
  esac
fi

bash "$ROOT/scripts/validate-env.sh" --profile "$PROFILE" --env-file "$ROOT/.env" || true
bash "$ROOT/scripts/lib/wait-stack.sh"

echo ""
echo "Streamclone is running at http://localhost:8090"
echo "Stop:  scripts/stop-streamclone.sh"
case "$PROFILE" in
  clipper|full)
    echo "Clips: make twitch-local-auth  (one-time Twitch login)"
    ;;
esac

if [ "$NO_BROWSER" = false ]; then
  if command -v xdg-open >/dev/null 2>&1; then
    xdg-open 'http://localhost:8090/' >/dev/null 2>&1 || true
  elif command -v open >/dev/null 2>&1; then
    open 'http://localhost:8090/' || true
  fi
fi
