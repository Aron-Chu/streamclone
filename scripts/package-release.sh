#!/usr/bin/env bash
# Build a desktop release bundle (compose + scripts, no app source).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-}"
if [ -z "$VERSION" ] && [ -f "$ROOT/VERSION" ]; then
  VERSION="$(tr -d '[:space:]' <"$ROOT/VERSION")"
fi
if [ -z "$VERSION" ]; then
  if git describe --tags --exact-match >/dev/null 2>&1; then
    VERSION="$(git describe --tags --exact-match)"
  elif git describe --tags --always >/dev/null 2>&1; then
    VERSION="$(git describe --tags --always)"
  else
    VERSION="dev"
  fi
fi

STAGE="$ROOT/dist/streamclone-$VERSION"
rm -rf "$STAGE"
mkdir -p "$STAGE"

copy_tree() {
  local src="$1"
  local dst="$2"
  mkdir -p "$dst"
  cp -a "$src"/. "$dst"/
}

echo "Packaging streamclone $VERSION -> dist/"

copy_tree "$ROOT/deploy" "$STAGE/deploy"
copy_tree "$ROOT/scripts" "$STAGE/scripts"
copy_tree "$ROOT/migrations" "$STAGE/migrations"
copy_tree "$ROOT/launchers" "$STAGE/launchers"
cp "$ROOT/VERSION" "$STAGE/VERSION"
cp "$ROOT/.env.dev" "$STAGE/.env.dev"
cp "$ROOT/.env.example" "$STAGE/.env.example"
cp "$ROOT/LICENSE" "$STAGE/LICENSE"
cp "$ROOT/Install Streamclone.cmd" "$STAGE/Install Streamclone.cmd"
cp "$ROOT/Check Streamclone.cmd" "$STAGE/Check Streamclone.cmd"
cp "$ROOT/Start Streamclone.cmd" "$STAGE/Start Streamclone.cmd"
cp "$ROOT/Stop Streamclone.cmd" "$STAGE/Stop Streamclone.cmd"
cp "$ROOT/Manage Streamclone.cmd" "$STAGE/Manage Streamclone.cmd"
cp "$ROOT/Uninstall Streamclone.cmd" "$STAGE/Uninstall Streamclone.cmd"

mkdir -p "$STAGE/deploy/env"
cat >"$STAGE/deploy/env/release-bundle.env" <<EOF
# Auto-generated release bundle — pull GHCR images pinned to this tag
IMAGE_TAG=$VERSION
STREAMCLONE_USE_IMAGES=1
SCRAPER_USE_IMAGES=1
# Loopback token-import/device-code endpoints are dev-only (docs/security.md).
TWITCH_DEV_TOKEN_IMPORT_ENABLED=false
EOF

if [ -n "${TWITCH_OAUTH_CLIENT_ID:-}" ] && [ -n "${TWITCH_OAUTH_CLIENT_SECRET:-}" ]; then
  cat >"$STAGE/deploy/env/oauth-bundle.env" <<EOF
TWITCH_OAUTH_CLIENT_ID=$TWITCH_OAUTH_CLIENT_ID
TWITCH_OAUTH_CLIENT_SECRET=$TWITCH_OAUTH_CLIENT_SECRET
EOF
  echo "Included oauth-bundle.env from release secrets"
fi
cp "$ROOT/deploy/env/oauth-bundle.env.example" "$STAGE/deploy/env/oauth-bundle.env.example"
mkdir -p "$STAGE/runtime"
cp "$ROOT/runtime/.gitkeep" "$STAGE/runtime/.gitkeep" 2>/dev/null || true

printf '%s' "$VERSION" >"$STAGE/VERSION"

cat >"$STAGE/README-quickstart.md" <<'EOF'
# Streamclone quick start

1. Install Docker Desktop and start it.
2. First time: run **Streamclone-Setup-*.exe** from [Releases](https://github.com/Aron-Chu/streamclone/releases/latest), or double-click **Install Streamclone.cmd** (~3–5 min)
3. Every day: double-click **Start Streamclone.cmd**
4. Open http://localhost:8090/

Stop (pause, keep data): **Stop Streamclone.cmd** / **Stop Streamclone.command**

Remove everything: **Uninstall Streamclone.cmd** / **Uninstall Streamclone.command**

Guide: https://github.com/Aron-Chu/streamclone/blob/master/docs/install-desktop.md
EOF

chmod +x "$STAGE/scripts"/*.sh 2>/dev/null || true
chmod +x "$STAGE/launchers"/*.sh "$STAGE/launchers"/*.command 2>/dev/null || true

mkdir -p "$ROOT/dist"
rm -f "$ROOT/dist/streamclone-${VERSION}-windows.zip" "$ROOT/dist/streamclone-${VERSION}.tar.gz"
# Archives place bundle files at archive root (not an extra parent folder) for install extract.
(
  cd "$STAGE"
  if command -v zip >/dev/null 2>&1; then
    zip -rq "$ROOT/dist/streamclone-${VERSION}-windows.zip" .
  else
    echo "zip not found; skipping windows archive" >&2
  fi
  tar -czf "$ROOT/dist/streamclone-${VERSION}.tar.gz" .
)

INSTALLER_SHA=""
INSTALLER_PATH="$ROOT/dist/Streamclone-Setup-${VERSION}.exe"
if [ -f "$INSTALLER_PATH" ]; then
  if command -v sha256sum >/dev/null 2>&1; then
    INSTALLER_SHA="$(sha256sum "$INSTALLER_PATH" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    INSTALLER_SHA="$(shasum -a 256 "$INSTALLER_PATH" | awk '{print $1}')"
  fi
fi

MANIFEST="$ROOT/dist/release-manifest.json"
cat >"$MANIFEST" <<EOF
{
  "appVersion": "$VERSION",
  "imageTag": "$VERSION",
  "composeFiles": [
    "deploy/docker-compose.yml",
    "deploy/docker-compose.local-tunnel.yml",
    "deploy/docker-compose.release.yml"
  ],
  "bundlePath": "dist/streamclone-${VERSION}",
  "installer": {
    "path": "dist/Streamclone-Setup-${VERSION}.exe",
    "sha256": "$INSTALLER_SHA"
  },
  "generatedAt": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

echo "Created:"
ls -la "$ROOT/dist/streamclone-${VERSION}"* 2>/dev/null || ls -la "$ROOT/dist/"
echo "Release manifest: $MANIFEST"
