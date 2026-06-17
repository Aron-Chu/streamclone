#!/usr/bin/env bash
# Register streamclone:// handler on macOS/Linux and set SETUP_CONTROL_WAKE_ENABLED=1.
set -euo pipefail

ROOT="${1:-}"
if [ -z "$ROOT" ]; then
  ROOT="$(cd "$(dirname "$0")/.." && pwd)"
fi
ROOT="$(cd "$ROOT" && pwd)"

if [ -f "$ROOT/.env" ]; then
  # shellcheck source=lib/env.sh
  source "$ROOT/scripts/lib/env.sh"
  env_set_key "$ROOT/.env" SETUP_CONTROL_WAKE_ENABLED 1
  if command -v docker >/dev/null 2>&1; then
    profile="$(env_read_value "$ROOT/.env" STREAMCLONE_PROFILE || true)"
    profile="${profile:-core}"
    compose_args=(compose --env-file "$ROOT/.env" -f "$ROOT/deploy/docker-compose.yml" -f "$ROOT/deploy/docker-compose.local-tunnel.yml")
    if [ -f "$ROOT/VERSION" ] || grep -q '^STREAMCLONE_USE_IMAGES=1' "$ROOT/.env" 2>/dev/null; then
      compose_args+=(-f "$ROOT/deploy/docker-compose.release.yml")
    fi
    read -r -a profiles <<<"$(env_compose_profiles "$profile")"
    for p in "${profiles[@]}"; do
      [ -n "$p" ] && compose_args+=(--profile "$p")
    done
    (cd "$ROOT" && docker "${compose_args[@]}" up -d --no-deps --force-recreate frontend) >/dev/null 2>&1 || true
  fi
fi

case "$(uname -s)" in
  Darwin)
    template_app="$ROOT/deploy/installer/StreamcloneURLHandler.app"
    [ -d "$template_app" ] || exit 0
    dest="$HOME/Applications/Streamclone URL Handler.app"
    rm -rf "$dest"
    cp -R "$template_app" "$dest"
    exe="$dest/Contents/MacOS/streamclone-url-handler"
    if [ -f "$exe" ]; then
      if sed --version >/dev/null 2>&1; then
        sed -i "s|__STREAMCLONE_INSTALL_ROOT__|$ROOT|g" "$exe"
      else
        sed -i '' "s|__STREAMCLONE_INSTALL_ROOT__|$ROOT|g" "$exe"
      fi
      chmod +x "$exe"
    fi
    lsregister="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
    if [ -x "$lsregister" ]; then
      "$lsregister" -f "$dest" >/dev/null 2>&1 || true
    fi
    echo "Registered streamclone:// handler: $dest"
    ;;
  Linux)
    template="$ROOT/deploy/installer/streamclone-url.desktop"
    [ -f "$template" ] || exit 0
    apps_dir="$HOME/.local/share/applications"
    mkdir -p "$apps_dir"
    sed "s|__STREAMCLONE_INSTALL_ROOT__|$ROOT|g" "$template" >"$apps_dir/streamclone-url.desktop"
    if command -v xdg-mime >/dev/null 2>&1; then
      xdg-mime default streamclone-url.desktop x-scheme-handler/streamclone 2>/dev/null || true
    fi
    echo "Registered streamclone:// handler: $apps_dir/streamclone-url.desktop"
    ;;
esac
