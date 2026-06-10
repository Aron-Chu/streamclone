#!/usr/bin/env bash
# Check prerequisites for Streamclone (non-developer friendly).
set -euo pipefail

INSTALL_HINTS=false
QUIET=false

usage() {
  cat <<'EOF'
Usage: scripts/preflight-deps.sh [--install-hints] [--quiet]
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --install-hints) INSTALL_HINTS=true; shift ;;
    --quiet) QUIET=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

errors=0
warnings=0

log() {
  local status="$1"
  shift
  [ "$QUIET" = true ] && return 0
  case "$status" in
    ok) printf '[ok] %s\n' "$*" ;;
    warn) printf '[!!] %s\n' "$*" ;;
    fail) printf '[X] %s\n' "$*" ;;
    *) printf '[--] %s\n' "$*" ;;
  esac
}

port_free() {
  local port="$1"
  if command -v ss >/dev/null 2>&1; then
    ! ss -ltn 2>/dev/null | grep -q ":${port} "
  elif command -v lsof >/dev/null 2>&1; then
    ! lsof -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
  else
    return 0
  fi
}

[ "$QUIET" = false ] && {
  echo ""
  echo "Streamclone — dependency check"
  echo "──────────────────────────────"
}

if command -v git >/dev/null 2>&1; then
  log ok "Git is installed"
else
  log fail "Git is not installed"
  errors=$((errors + 1))
  [ "$INSTALL_HINTS" = true ] && echo "  Install: sudo apt install git  (Debian/Ubuntu) or brew install git (macOS)"
fi

if ! command -v docker >/dev/null 2>&1; then
  log fail "Docker is not installed or not on PATH"
  errors=$((errors + 1))
  [ "$INSTALL_HINTS" = true ] && cat <<'HINT'

  Linux (Docker Engine — no Desktop required):
    https://docs.docker.com/engine/install/

  macOS:
    Docker Desktop — https://docs.docker.com/desktop/setup/install/mac-install/

HINT
else
  log ok "Docker CLI found"
  if docker info >/dev/null 2>&1; then
    log ok "Docker engine is running"
  else
    log fail "Docker is installed but not running — start the Docker service"
    errors=$((errors + 1))
    [ "$INSTALL_HINTS" = true ] && echo "  Linux: sudo systemctl start docker"
  fi
  if docker compose version >/dev/null 2>&1; then
    log ok "Docker Compose v2 available"
  else
    log fail "docker compose is missing"
    errors=$((errors + 1))
  fi
fi

if port_free 8090; then
  log ok "Port 8090 is free (Streamclone proxy)"
else
  log warn "Port 8090 is already in use"
  warnings=$((warnings + 1))
fi

if command -v twitch >/dev/null 2>&1; then
  log ok "Twitch CLI found (optional — Clip Studio / chat login)"
else
  log warn "Twitch CLI not found — core viewing works without it"
  warnings=$((warnings + 1))
  [ "$INSTALL_HINTS" = true ] && echo "  Install: https://github.com/twitchdev/twitch-cli#installation"
fi

if [ "$QUIET" = false ]; then
  echo ""
  if [ "$errors" -gt 0 ]; then
    echo "preflight-deps: FAILED — fix $errors issue(s) above."
    exit 1
  fi
  echo "preflight-deps: OK — $warnings warning(s). Run: scripts/start-streamclone.sh"
fi
exit 0
