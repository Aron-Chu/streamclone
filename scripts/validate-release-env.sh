#!/usr/bin/env bash
# Assert release-bundle.env synthesizes expected desktop defaults (CI + local packaging).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${VERSION:-}"
if [ -z "$VERSION" ] && [ -f "$ROOT/VERSION" ]; then
  VERSION="$(tr -d '[:space:]' <"$ROOT/VERSION")"
fi
if [ -z "$VERSION" ]; then
  echo "validate-release-env: VERSION missing" >&2
  exit 1
fi

BUNDLE="${1:-$ROOT/dist/streamclone-$VERSION}"
if [ ! -d "$BUNDLE" ]; then
  echo "validate-release-env: bundle not found at $BUNDLE (run scripts/package-release.sh first)" >&2
  exit 1
fi

RELEASE_BUNDLE_ENV="$BUNDLE/deploy/env/release-bundle.env"
if [ ! -f "$RELEASE_BUNDLE_ENV" ]; then
  echo "validate-release-env: missing $RELEASE_BUNDLE_ENV" >&2
  exit 1
fi

if [ ! -f "$BUNDLE/deploy/env/oauth-bundle.env" ]; then
  echo "WARN: oauth-bundle.env missing in release bundle (Reddit-only Pulse Wire still works)" >&2
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
cp -a "$BUNDLE"/. "$TMP"/

cd "$TMP"
# shellcheck source=scripts/lib/env.sh
source "$TMP/scripts/lib/env.sh"

env_synthesize core "$TMP/.env"

assert_eq() {
  local key="$1"
  local expected="$2"
  local actual
  actual="$(env_read_value "$TMP/.env" "$key" || true)"
  if [ "$actual" != "$expected" ]; then
    echo "validate-release-env: expected $key=$expected, got '$actual'" >&2
    exit 1
  fi
}

assert_eq REDDIT_COMMERCIAL_OK true
assert_eq TWITCH_DEV_TOKEN_IMPORT_ENABLED false

image_tag="$(env_read_value "$TMP/.env" IMAGE_TAG || true)"
if [ "$image_tag" != "$VERSION" ]; then
  echo "validate-release-env: IMAGE_TAG ($image_tag) != VERSION ($VERSION)" >&2
  exit 1
fi

bundle_tag="$(env_read_value "$RELEASE_BUNDLE_ENV" IMAGE_TAG || true)"
if [ "$bundle_tag" != "$VERSION" ]; then
  echo "validate-release-env: release-bundle.env IMAGE_TAG ($bundle_tag) != VERSION ($VERSION)" >&2
  exit 1
fi

echo "validate-release-env: OK (core profile, IMAGE_TAG=$VERSION, Pulse Wire off in desktop bundle)"
