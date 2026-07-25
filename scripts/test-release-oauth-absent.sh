#!/usr/bin/env bash
# Prove release packaging never ships Twitch client secrets or oauth-bundle.env.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export VERSION="test-oauth-canary"
# Split canary so packaging of this script (if ever included) is less sensitive;
# test-* scripts are excluded from release bundles.
CANARY_SECRET="canary-client-secret-DO-NOT-SHIP"
export TWITCH_OAUTH_CLIENT_ID="canary-client-id-DO-NOT-SHIP"
export TWITCH_OAUTH_CLIENT_SECRET="$CANARY_SECRET"

bash scripts/package-release.sh

BUNDLE="dist/streamclone-${VERSION}"
[[ -d "$BUNDLE" ]] || { echo "missing bundle dir $BUNDLE" >&2; exit 1; }

if [[ -f "$BUNDLE/deploy/env/oauth-bundle.env" ]]; then
  echo "FAIL: oauth-bundle.env present despite canary secrets in env" >&2
  exit 1
fi

if [[ -f "$BUNDLE/scripts/test-release-oauth-absent.sh" ]]; then
  echo "FAIL: test scripts must not be packaged" >&2
  exit 1
fi

# Scan only env material operators would load — not docs/scripts prose.
if grep -R -n -E "${CANARY_SECRET}|TWITCH_OAUTH_CLIENT_SECRET=.+" \
  "$BUNDLE/deploy/env" "$BUNDLE/.env.example" 2>/dev/null \
  | grep -v 'oauth-bundle\.env\.example' | grep -q .; then
  echo "FAIL: secret canary found in release env material" >&2
  exit 1
fi

bash scripts/validate-release-env.sh "$BUNDLE"

# Negative: plant a forbidden file and ensure validator rejects.
cp "$BUNDLE/deploy/env/oauth-bundle.env.example" "$BUNDLE/deploy/env/oauth-bundle.env"
printf 'TWITCH_OAUTH_CLIENT_SECRET=%s\n' "$CANARY_SECRET" >>"$BUNDLE/deploy/env/oauth-bundle.env"
if bash scripts/validate-release-env.sh "$BUNDLE"; then
  echo "FAIL: validator accepted oauth-bundle.env" >&2
  exit 1
fi

echo "test-release-oauth-absent: OK"
