#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${STREAMCLONE_DIR:-$HOME/streamclone}"
PROFILE="core"
USE_RELEASE=false
NON_INTERACTIVE=false
USE_IMAGES=false
DESKTOP_SHORTCUT=false
VERSION=""

usage() {
  cat <<'EOF'
Usage: install.sh [options]

  --dir PATH              Install directory (default: ~/streamclone)
  --profile NAME          core|scraper|clipper|full (default: core)
  --release               Download latest release bundle instead of git clone
  --version TAG           Release tag (e.g. v1.0.0) with --release
  --use-images            Pull GHCR images (passes through to setup.sh)
  --non-interactive       No prompts
  --desktop-shortcut      Create Start/Stop launchers (default with --release --non-interactive)
  -h, --help              Show help
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --dir) INSTALL_DIR="$2"; shift 2 ;;
    --profile) PROFILE="$2"; shift 2 ;;
    --release) USE_RELEASE=true; shift ;;
    --version) VERSION="$2"; shift 2 ;;
    --use-images) USE_IMAGES=true; shift ;;
    --non-interactive) NON_INTERACTIVE=true; shift ;;
    --desktop-shortcut) DESKTOP_SHORTCUT=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

bash "$(dirname "$0")/preflight-deps.sh" --install-hints

if [ "$USE_RELEASE" = true ]; then
  [ "$USE_IMAGES" = false ] && USE_IMAGES=true
  [ "$NON_INTERACTIVE" = false ] && NON_INTERACTIVE=true
  if [ "$DESKTOP_SHORTCUT" = false ] && [ "$NON_INTERACTIVE" = true ]; then
    DESKTOP_SHORTCUT=true
  fi
  dl_args=("$INSTALL_DIR")
  [ -n "$VERSION" ] && dl_args+=("$VERSION")
  bash "$(dirname "$0")/lib/release-download.sh" "${dl_args[@]}"
else
  if [ -d "$INSTALL_DIR/.git" ]; then
    echo "Updating existing checkout at $INSTALL_DIR..."
    git -C "$INSTALL_DIR" pull --ff-only
  else
    echo "Cloning Streamclone to $INSTALL_DIR..."
    git clone https://github.com/Aron-Chu/streamclone.git "$INSTALL_DIR"
  fi
fi

setup_args=(--profile "$PROFILE")
[ "$NON_INTERACTIVE" = true ] && setup_args+=(--non-interactive)
[ "$USE_IMAGES" = true ] && setup_args+=(--use-images)

bash "$INSTALL_DIR/scripts/setup.sh" "${setup_args[@]}"

if [ "$DESKTOP_SHORTCUT" = true ]; then
  bash "$INSTALL_DIR/scripts/install-desktop-shortcut.sh" "$INSTALL_DIR"
fi

cat <<EOF

Installed at: $INSTALL_DIR
Open:         http://localhost:8090
Start:        bash $INSTALL_DIR/scripts/start-streamclone.sh
Stop stack:   bash $INSTALL_DIR/scripts/stop-streamclone.sh
EOF
