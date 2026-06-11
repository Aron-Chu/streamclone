#!/usr/bin/env bash
# Build a desktop release bundle (compose + scripts, no app source).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-}"
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
cp "$ROOT/.env.dev" "$STAGE/.env.dev"
cp "$ROOT/.env.example" "$STAGE/.env.example"
cp "$ROOT/Install Streamclone.cmd" "$STAGE/Install Streamclone.cmd"
cp "$ROOT/Start Streamclone.cmd" "$STAGE/Start Streamclone.cmd"
cp "$ROOT/Stop Streamclone.cmd" "$STAGE/Stop Streamclone.cmd"

mkdir -p "$STAGE/deploy/env"
cat >"$STAGE/deploy/env/release-bundle.env" <<EOF
# Auto-generated release bundle — pull GHCR images pinned to this tag
IMAGE_TAG=$VERSION
STREAMCLONE_USE_IMAGES=1
EOF

printf '%s' "$VERSION" >"$STAGE/VERSION"

cat >"$STAGE/README-quickstart.md" <<'EOF'
# Streamclone quick start

1. Install Docker Desktop and start it.
2. First time: double-click **Install Streamclone.cmd** (~3–5 min)
3. Every day: double-click **Start Streamclone.cmd**
4. Open http://localhost:8090/welcome

Stop: **Stop Streamclone.cmd** (Windows) or **Stop Streamclone.command** (macOS)

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

echo "Created:"
ls -la "$ROOT/dist/streamclone-${VERSION}"* 2>/dev/null || ls -la "$ROOT/dist/"
