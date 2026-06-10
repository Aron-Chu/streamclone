#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${1:-${STREAMCLONE_DIR:-$HOME/streamclone}}"
ROOT="$(cd "$INSTALL_DIR" && pwd)"
REPO_LAUNCHERS="$(cd "$(dirname "$0")/../launchers" && pwd)"
LAUNCHERS="$ROOT/launchers"

mkdir -p "$LAUNCHERS"

if [ -d "$REPO_LAUNCHERS" ]; then
  for f in install-streamclone-launcher.sh "Install Streamclone.command" "Start Streamclone.command" "Stop Streamclone.command"; do
    [ -f "$REPO_LAUNCHERS/$f" ] && cp "$REPO_LAUNCHERS/$f" "$LAUNCHERS/$f"
  done
  chmod +x "$LAUNCHERS"/*.sh "$LAUNCHERS"/*.command 2>/dev/null || true
else
  cat >"$LAUNCHERS/Start Streamclone.command" <<EOF
#!/usr/bin/env bash
cd "$ROOT"
exec bash "$ROOT/scripts/start-streamclone.sh"
EOF
  cat >"$LAUNCHERS/Stop Streamclone.command" <<EOF
#!/usr/bin/env bash
cd "$ROOT"
exec bash "$ROOT/scripts/stop-streamclone.sh"
EOF
  chmod +x "$LAUNCHERS/Start Streamclone.command" "$LAUNCHERS/Stop Streamclone.command"
fi

APPS_DIR="$HOME/Applications"
mkdir -p "$APPS_DIR"
ln -sf "$LAUNCHERS/Start Streamclone.command" "$APPS_DIR/Streamclone Start.command"
ln -sf "$LAUNCHERS/Stop Streamclone.command" "$APPS_DIR/Streamclone Stop.command"
ln -sf "$LAUNCHERS/Install Streamclone.command" "$APPS_DIR/Streamclone Install.command" 2>/dev/null || true

echo "Launchers: $LAUNCHERS"
echo "macOS shortcuts: $APPS_DIR/Streamclone Start.command"
