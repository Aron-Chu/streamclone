#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${1:?install dir required}"
VERSION="${2:-}"
REPO="${STREAMCLONE_GITHUB_REPO:-Aron-Chu/streamclone}"

api() {
  curl -fsSL -H 'User-Agent: streamclone-install' "$1"
}

if [ -n "$VERSION" ]; then
  release_json="$(api "https://api.github.com/repos/$REPO/releases/tags/$VERSION")"
else
  release_json="$(api "https://api.github.com/repos/$REPO/releases/latest")"
fi

tag="$(printf '%s' "$release_json" | grep -m1 '"tag_name"' | sed 's/.*"tag_name": "\([^"]*\)".*/\1/')"
url="$(printf '%s' "$release_json" | grep -o 'https://[^"]*\.tar\.gz' | head -1)"
name="$(printf '%s' "$release_json" | grep -o '"name": "streamclone[^"]*\.tar\.gz"' | head -1 | sed 's/"name": "\(.*\)"/\1/')"

if [ -z "$url" ]; then
  echo "No .tar.gz release asset found for $tag" >&2
  exit 1
fi

echo "Downloading ${name:-bundle} ($tag)..."
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

curl -fsSL -o "$tmpdir/bundle.tar.gz" "$url"
rm -rf "$INSTALL_DIR"
mkdir -p "$INSTALL_DIR"
tar -xzf "$tmpdir/bundle.tar.gz" -C "$INSTALL_DIR"

# Legacy archives nested streamclone-<tag>/ — hoist if needed.
nested="$INSTALL_DIR/streamclone-$tag"
if [ -d "$nested" ] && [ ! -f "$INSTALL_DIR/VERSION" ]; then
  shopt -s dotglob
  mv "$nested"/* "$INSTALL_DIR"/
  shopt -u dotglob
  rmdir "$nested" 2>/dev/null || rm -rf "$nested"
fi

if [ ! -f "$INSTALL_DIR/VERSION" ]; then
  echo "Release extract failed — VERSION missing in $INSTALL_DIR" >&2
  exit 1
fi

echo "Installed release bundle to $INSTALL_DIR"
echo "$tag"
