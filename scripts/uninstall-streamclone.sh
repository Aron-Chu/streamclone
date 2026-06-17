#!/usr/bin/env bash
# Complete Streamclone uninstall — stop stack, remove volumes, config, shortcuts, install folder.
set -euo pipefail

INSTALL_DIR="${STREAMCLONE_DIR:-}"
NONINTERACTIVE=0
PRUNE_IMAGES=0
KEEP_INSTALL_DIR=0

usage() {
  echo "Usage: uninstall-streamclone.sh [--install-dir PATH] [--non-interactive] [--prune-images] [--keep-install-dir]" >&2
  exit 1
}

while [ $# -gt 0 ]; do
  case "$1" in
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --non-interactive) NONINTERACTIVE=1; shift ;;
    --prune-images) PRUNE_IMAGES=1; shift ;;
    --keep-install-dir) KEEP_INSTALL_DIR=1; shift ;;
    -h|--help) usage ;;
    *) echo "Unknown option: $1" >&2; usage ;;
  esac
done

resolve_root() {
  local candidate
  for candidate in \
    "${INSTALL_DIR:-}" \
    "$(cd "$(dirname "$0")/.." && pwd)" \
    "${HOME}/streamclone"; do
    [ -n "$candidate" ] || continue
    if [ -f "$candidate/scripts/start-streamclone.sh" ]; then
      printf '%s' "$(cd "$candidate" && pwd)"
      return 0
    fi
  done
  echo "Streamclone install not found." >&2
  return 1
}

use_release_images() {
  local root="$1"
  [ -f "$root/VERSION" ] && return 0
  [ -f "$root/deploy/env/release-bundle.env" ] && return 0
  if [ -f "$root/.env" ] && grep -q '^STREAMCLONE_USE_IMAGES=1' "$root/.env" 2>/dev/null; then
    return 0
  fi
  return 1
}

compose_down() {
  local root="$1"
  local volumes_flag="$2"
  local env_file="$root/.env"
  [ -f "$env_file" ] || return 0
  cd "$root"

  local stop_script="$root/scripts/stop-streamclone.sh"
  if [ -f "$stop_script" ]; then
    bash "$stop_script" 2>/dev/null || true
    sleep 2
  fi

  local -a compose_args=(compose --env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml)
  if use_release_images "$root"; then
    compose_args+=(-f deploy/docker-compose.release.yml)
  fi
  local -a down_args=(down --remove-orphans --timeout 30)
  [ "$volumes_flag" = "1" ] && down_args+=(-v)

  echo "Stopping Docker stack..."
  docker "${compose_args[@]}" --profile scraper --profile clipper "${down_args[@]}" 2>/dev/null || true
  docker "${compose_args[@]}" "${down_args[@]}" 2>/dev/null || true
}

ROOT="$(resolve_root)"

echo ""
echo "Streamclone — Complete uninstall"
echo "================================"
echo "Install folder: $ROOT"
echo ""
echo "This will:"
echo "  - Stop all Streamclone Docker containers"
echo "  - Delete Docker volumes (database, MinIO, clipper data)"
echo "  - Remove .env and local secrets"
echo "  - Remove Desktop / macOS shortcuts"
[ "$PRUNE_IMAGES" = "1" ] && echo "  - Remove downloaded ghcr.io/aron-chu/streamclone images"
[ "$KEEP_INSTALL_DIR" = "0" ] && echo "  - Delete the install folder"
echo ""

if [ "$NONINTERACTIVE" != "1" ]; then
  read -r -p "Type YES to continue: " ans
  if [ "$ans" != "YES" ]; then
    echo "Uninstall cancelled."
    exit 0
  fi
fi

control_pid_file="$ROOT/.streamclone-setup-control.pid"
if [ -f "$control_pid_file" ]; then
  pid="$(tr -d '[:space:]' <"$control_pid_file")"
  if [[ "$pid" =~ ^[0-9]+$ ]]; then
    kill -9 "$pid" 2>/dev/null || true
  fi
  rm -f "$control_pid_file"
fi

compose_down "$ROOT" 1

if [ "$PRUNE_IMAGES" = "1" ]; then
  tag="latest"
  [ -f "$ROOT/VERSION" ] && tag="$(tr -d '[:space:]' <"$ROOT/VERSION")"
  if [ -f "$ROOT/.env" ]; then
    maybe="$(grep '^IMAGE_TAG=' "$ROOT/.env" 2>/dev/null | cut -d= -f2- || true)"
    [ -n "$maybe" ] && tag="$maybe"
  fi
  echo "Pruning GHCR images (tag: $tag)..."
  for repo in metadata video chat analytics emote frontend clipper; do
    docker image rm -f "ghcr.io/aron-chu/streamclone/${repo}:${tag}" 2>/dev/null || true
  done
fi

for name in .env .streamclone-profile .streamclone-setup-control.pid; do
  if [ -e "$ROOT/$name" ]; then
    rm -f "$ROOT/$name"
    echo "Removed $name"
  fi
done

APPS_DIR="${HOME}/Applications"
for name in "Streamclone Start.command" "Streamclone Stop.command" "Streamclone Install.command" \
  "Streamclone Manage.command" "Streamclone Check.command" "Streamclone Uninstall.command"; do
  if [ -e "$APPS_DIR/$name" ]; then
    rm -f "$APPS_DIR/$name"
    echo "Removed $APPS_DIR/$name"
  fi
done

if [ -d "$APPS_DIR/Streamclone URL Handler.app" ]; then
  rm -rf "$APPS_DIR/Streamclone URL Handler.app"
  echo "Removed $APPS_DIR/Streamclone URL Handler.app"
fi

linux_desktop="${HOME}/.local/share/applications/streamclone-url.desktop"
if [ -f "$linux_desktop" ]; then
  rm -f "$linux_desktop"
  echo "Removed $linux_desktop"
fi

if command -v powershell.exe >/dev/null 2>&1; then
  powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$ROOT/scripts/unregister-setup-control-protocol.ps1" 2>/dev/null || true
elif command -v pwsh >/dev/null 2>&1; then
  pwsh -NoProfile -ExecutionPolicy Bypass -File "$ROOT/scripts/unregister-setup-control-protocol.ps1" 2>/dev/null || true
fi

if [ "$KEEP_INSTALL_DIR" = "1" ]; then
  echo ""
  echo "Uninstall complete (install folder kept): $ROOT"
  exit 0
fi

(
  sleep 2
  rm -rf "$ROOT"
) &
echo ""
echo "Uninstall complete. Install folder will be removed shortly."
echo "If $ROOT remains, close terminals/File Explorer in that folder and delete manually."
