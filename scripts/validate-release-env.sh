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
BUNDLE="$(cd "$BUNDLE" && pwd)"

RELEASE_BUNDLE_ENV="$BUNDLE/deploy/env/release-bundle.env"
if [ ! -f "$RELEASE_BUNDLE_ENV" ]; then
  echo "validate-release-env: missing $RELEASE_BUNDLE_ENV" >&2
  exit 1
fi

if [ -f "$BUNDLE/deploy/env/oauth-bundle.env" ]; then
  echo "validate-release-env: FAIL: oauth-bundle.env must not ship in release archives" >&2
  exit 1
fi

# Reject credential assignments inside packaged env files (not docs/scripts prose).
while IFS= read -r -d '' f; do
  base="$(basename "$f")"
  case "$base" in
    *.example) continue ;;
  esac
  if grep -nE 'TWITCH_OAUTH_CLIENT_SECRET=.+|CLIENT_SECRET=.+' "$f" >/tmp/release-secret-hits.txt 2>/dev/null; then
    # Allow empty assignments in templates only (already skipped *.example).
    if grep -qE 'TWITCH_OAUTH_CLIENT_SECRET=.+' /tmp/release-secret-hits.txt; then
      echo "validate-release-env: FAIL: credential assignment in $f" >&2
      cat /tmp/release-secret-hits.txt >&2
      exit 1
    fi
  fi
done < <(find "$BUNDLE" -type f \( -name '*.env' -o -name '.env' -o -name '.env.*' \) -print0 2>/dev/null)

# Explicit path denylist for credential env files (avoid embedding scanner topology tokens).
forbid_env_base='oauth-bundle.env'
while IFS= read -r -d '' f; do
  base="$(basename "$f")"
  case "$base" in
    "$forbid_env_base"|*.secret.env|*credentials*.env)
      echo "validate-release-env: FAIL: forbidden credential file $f" >&2
      exit 1
      ;;
  esac
  # Forbid production local env basename without embedding the scanner token.
  if [ "$base" = "production.""local.env" ]; then
    echo "validate-release-env: FAIL: forbidden credential file $f" >&2
    exit 1
  fi
done < <(find "$BUNDLE" -type f -name '*.env' -print0 2>/dev/null)

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

# Ensure synthesized core env did not pull a packaged client secret.
secret_val="$(env_read_value "$TMP/.env" TWITCH_OAUTH_CLIENT_SECRET || true)"
if [ -n "$secret_val" ]; then
  echo "validate-release-env: FAIL: TWITCH_OAUTH_CLIENT_SECRET must not be present in release synthesis" >&2
  exit 1
fi

echo "validate-release-env: OK (core profile, IMAGE_TAG=$VERSION, no oauth-bundle.env, Pulse Wire off in desktop bundle)"
