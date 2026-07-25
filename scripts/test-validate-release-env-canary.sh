#!/usr/bin/env bash
# Lightweight PR-CI canary: prove credential-bearing env files are rejected
# without running full package-release / env synthesis (Actions billing).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

STAGE="$(mktemp -d "${TMPDIR:-/tmp}/streamclone-oauth-canary.XXXXXX")"
cleanup() { rm -rf "$STAGE"; }
trap cleanup EXIT

mkdir -p "$STAGE/deploy/env"
cp "$ROOT/deploy/env/oauth-bundle.env.example" "$STAGE/deploy/env/oauth-bundle.env.example"

# Mirror the forbidden-path checks from validate-release-env.sh (secret rejection only).
reject_forbidden_env() {
  local bundle="$1"
  if [[ -f "$bundle/deploy/env/oauth-bundle.env" ]]; then
    echo "reject: oauth-bundle.env present" >&2
    return 1
  fi
  while IFS= read -r -d '' f; do
    base="$(basename "$f")"
    case "$base" in
      oauth-bundle.env|*.secret.env|*credentials*.env)
        echo "reject: forbidden credential file $f" >&2
        return 1
        ;;
    esac
    case "$base" in
      *.example) continue ;;
    esac
    if grep -qE 'TWITCH_OAUTH_CLIENT_SECRET=.+' "$f" 2>/dev/null; then
      echo "reject: credential assignment in $f" >&2
      return 1
    fi
  done < <(find "$bundle" -type f -name '*.env' -print0 2>/dev/null)
  return 0
}

# Positive: example-only tree is clean.
reject_forbidden_env "$STAGE"

# Negative: planted oauth-bundle.env must fail.
CANARY_SECRET="canary-client-secret-DO-NOT-SHIP"
cp "$STAGE/deploy/env/oauth-bundle.env.example" "$STAGE/deploy/env/oauth-bundle.env"
printf 'TWITCH_OAUTH_CLIENT_SECRET=%s\n' "$CANARY_SECRET" >>"$STAGE/deploy/env/oauth-bundle.env"
if reject_forbidden_env "$STAGE"; then
  echo "FAIL: oauth-bundle.env was accepted" >&2
  exit 1
fi
rm -f "$STAGE/deploy/env/oauth-bundle.env"

# Negative: credentials*.env basename must fail.
printf 'TWITCH_OAUTH_CLIENT_SECRET=%s\n' "$CANARY_SECRET" >"$STAGE/deploy/env/operator-credentials.env"
if reject_forbidden_env "$STAGE"; then
  echo "FAIL: *credentials*.env was accepted" >&2
  exit 1
fi
rm -f "$STAGE/deploy/env/operator-credentials.env"

# Negative: secret assignment in a non-example env must fail.
printf 'TWITCH_OAUTH_CLIENT_SECRET=%s\n' "$CANARY_SECRET" >"$STAGE/deploy/env/extra.env"
if reject_forbidden_env "$STAGE"; then
  echo "FAIL: CLIENT_SECRET assignment was accepted" >&2
  exit 1
fi

echo "test-validate-release-env-canary: OK"
