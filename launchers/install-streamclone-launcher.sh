#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-install}"
LAUNCHER_ROOT="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$LAUNCHER_ROOT/.." && pwd)"

if [ ! -f "$ROOT/scripts/start-streamclone.sh" ]; then
  ROOT="${STREAMCLONE_DIR:-$HOME/streamclone}"
fi

case "$ACTION" in
  install)
    INSTALL_SH="$ROOT/scripts/install.sh"
    if [ ! -f "$INSTALL_SH" ]; then
      echo "Downloading installer..."
      tmp="$(mktemp)"
      curl -fsSL -o "$tmp" https://raw.githubusercontent.com/Aron-Chu/streamclone/master/scripts/install.sh
      bash "$tmp" --release --non-interactive --use-images --desktop-shortcut
      rm -f "$tmp"
    else
      bash "$INSTALL_SH" --release --non-interactive --use-images --desktop-shortcut
    fi
    ;;
  start)
    exec bash "$ROOT/scripts/start-streamclone.sh"
    ;;
  stop)
    echo "Stopping Docker stack (config and data are kept)..."
    exec bash "$ROOT/scripts/stop-streamclone.sh"
    ;;
  uninstall)
    exec bash "$ROOT/scripts/uninstall-streamclone.sh" --install-dir "$ROOT"
    ;;
  *)
    echo "Usage: install-streamclone-launcher.sh install|start|stop|uninstall" >&2
    exit 1
    ;;
esac
