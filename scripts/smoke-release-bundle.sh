#!/usr/bin/env bash
# Release bundle verification smoke (moment-timeline Requirement 33, P0).
#
# Trust-critical: catches deployments where the served frontend bundle is stale
# versus the CI-built artifact. The classic failure signature is "live playback
# works but VOD fails with HTTP 400 invalid_vod_id for a well-formed id" — the
# stale browser bundle posts the wrong field (vodId instead of vod_id), so the
# backend never sees a valid id. This is a frontend deploy mismatch, NOT a
# Twitch content issue.
#
# Checks:
#   A. Record the served frontend entry script (e.g. /assets/index-*.js from
#      index.html) and compare it to the CI-built artifact (RELEASE_BUNDLE_ENTRY,
#      or the entry referenced by a locally built frontend/dist/index.html).
#      A mismatch fails as a deploy mismatch (Req 33.1).
#   B. POST /v1/stream/vod/start with a known well-formed VOD_Identifier
#      (^\d{5,20}$). HTTP 400 invalid_vod_id for a valid id indicates a client
#      bundle regression and fails (Req 33.2).
#   C. Classify live-pass / VOD-fail-on-400 as a frontend deploy mismatch rather
#      than a Twitch content issue (Req 33.3).
#
# The stack may not be running. By default an unreachable stack is reported as a
# SKIP (exit 0). Set REQUIRE_STACK=1 to make stack-down a hard failure.
#
# Usage:
#   bash scripts/smoke-release-bundle.sh [--require-stack] [--strict]
# Env:
#   SMOKE_BASE_URL (default http://localhost:8090)
#   SMOKE_VOD_ID   (default 1234567890, must match ^\d{5,20}$)
#   RELEASE_BUNDLE_ENTRY  explicit expected entry filename or sha256
#   RELEASE_BUNDLE_DIST   dir holding the CI-built index.html (default frontend/dist)
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BASE_URL="${SMOKE_BASE_URL:-http://localhost:8090}"
BASE_URL="${BASE_URL%/}"
VOD_ID="${SMOKE_VOD_ID:-1234567890}"
EXPECTED_ENTRY="${RELEASE_BUNDLE_ENTRY:-}"
DIST_DIR="${RELEASE_BUNDLE_DIST:-frontend/dist}"
REQUIRE_STACK="${REQUIRE_STACK:-0}"
STRICT="${STRICT:-0}"

for arg in "$@"; do
  case "$arg" in
    --require-stack) REQUIRE_STACK=1 ;;
    --strict) STRICT=1 ;;
  esac
done

FAILURES=0

step() { echo "smoke-release-bundle: $1"; }
add_fail() { echo "  FAIL: $1" >&2; FAILURES=$((FAILURES + 1)); }
ok()   { echo "  ok: $1"; }
note() { echo "  note: $1"; }

# Extract the ES-module entry script src from an index.html document on stdin.
entry_script() {
  grep -oE '<script[^>]*type="module"[^>]*src="[^"]+"|<script[^>]*src="[^"]+"[^>]*type="module"' \
    | grep -oE 'src="[^"]+"' | head -n1 | sed -E 's/^src="(.*)"$/\1/'
}
basename_of() { printf '%s' "${1##*/}"; }

# --- Reachability guard -----------------------------------------------------
step "probing $BASE_URL/ ..."
INDEX_HTML="$(curl -fsS --max-time 10 "$BASE_URL/" 2>/dev/null)"
if [ -z "$INDEX_HTML" ]; then
  msg="stack not reachable at $BASE_URL/"
  if [ "$REQUIRE_STACK" = "1" ]; then
    echo "smoke-release-bundle: $msg" >&2
    echo "smoke-release-bundle: --require-stack set -> FAIL" >&2
    exit 1
  fi
  echo "smoke-release-bundle: SKIP - $msg"
  echo "smoke-release-bundle: bring the stack up (make up) then re-run, or set REQUIRE_STACK=1 in a release gate."
  exit 0
fi
ok "frontend index reachable"
LIVE_PASS=1

# --- Check A: served entry script vs CI-built artifact (Req 33.1) -----------
step "checking served frontend entry script (Req 33.1) ..."
SERVED_ENTRY="$(printf '%s' "$INDEX_HTML" | entry_script)"
if [ -z "$SERVED_ENTRY" ]; then
  add_fail "could not locate an ES-module entry <script> in served index.html"
else
  SERVED_NAME="$(basename_of "$SERVED_ENTRY")"
  note "served entry: $SERVED_ENTRY"
  case "$SERVED_ENTRY" in
    *main.tsx) note "served entry is the dev source (main.tsx); looks like a Vite dev server, not a built bundle." ;;
  esac

  # Record a content hash of the served entry for the deploy record.
  SERVED_HASH=""
  case "$SERVED_ENTRY" in
    http*://*) ENTRY_URL="$SERVED_ENTRY" ;;
    /*) ENTRY_URL="$BASE_URL$SERVED_ENTRY" ;;
    *)  ENTRY_URL="$BASE_URL/$SERVED_ENTRY" ;;
  esac
  if JS="$(curl -fsS --max-time 15 "$ENTRY_URL" 2>/dev/null)"; then
    if command -v sha256sum >/dev/null 2>&1; then
      SERVED_HASH="$(printf '%s' "$JS" | sha256sum | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
      SERVED_HASH="$(printf '%s' "$JS" | shasum -a 256 | awk '{print $1}')"
    fi
    [ -n "$SERVED_HASH" ] && note "served entry sha256: $SERVED_HASH"
  else
    note "could not fetch served entry for hashing; comparing by filename only"
  fi

  # Resolve the expected entry (CI-built artifact).
  EXPECTED="$EXPECTED_ENTRY"
  EXPECTED_SOURCE="RELEASE_BUNDLE_ENTRY"
  if [ -z "$EXPECTED" ] && [ -f "$DIST_DIR/index.html" ]; then
    EXPECTED="$(entry_script < "$DIST_DIR/index.html")"
    EXPECTED_SOURCE="$DIST_DIR/index.html"
  fi

  if [ -z "$EXPECTED" ]; then
    n="no expected entry available (set RELEASE_BUNDLE_ENTRY or build $DIST_DIR) - recorded served entry only"
    if [ "$STRICT" = "1" ]; then add_fail "$n"; else note "$n"; fi
  else
    EXPECTED_NAME="$(basename_of "$EXPECTED")"
    note "expected entry: $EXPECTED (from $EXPECTED_SOURCE)"
    if [ -n "$EXPECTED_ENTRY" ] && [ -n "$SERVED_HASH" ] && [ "$EXPECTED_ENTRY" = "$SERVED_HASH" ]; then
      ok "served entry sha256 matches expected hash"
    elif [ "$SERVED_NAME" = "$EXPECTED_NAME" ]; then
      ok "served entry filename matches CI-built artifact ($SERVED_NAME)"
    else
      add_fail "served entry '$SERVED_NAME' != CI-built artifact '$EXPECTED_NAME' - DEPLOY MISMATCH (stale frontend bundle)"
    fi
  fi
fi

# --- Check B: VOD start round-trip with a valid id (Req 33.2) ---------------
step "checking VOD start with valid id '$VOD_ID' (Req 33.2) ..."
if ! printf '%s' "$VOD_ID" | grep -qE '^[0-9]{5,20}$'; then
  add_fail "configured SMOKE_VOD_ID '$VOD_ID' is not a well-formed VOD_Identifier (^\\d{5,20}\$)"
fi
VOD_FAIL_REGRESSION=0
HTTP_BODY="$(curl -sS --max-time 20 -o - -w $'\n%{http_code}' \
  -X POST "$BASE_URL/v1/stream/vod/start" \
  -H 'Content-Type: application/json' \
  -d "{\"vod_id\":\"$VOD_ID\",\"offset_seconds\":0}" 2>/dev/null)"
VOD_STATUS="$(printf '%s' "$HTTP_BODY" | tail -n1)"
VOD_RESP="$(printf '%s' "$HTTP_BODY" | sed '$d')"
VOD_CODE="$(printf '%s' "$VOD_RESP" | grep -oE '"(error|code)"[[:space:]]*:[[:space:]]*"[^"]+"' | head -n1 | sed -E 's/.*"([^"]+)"$/\1/')"

if [ -z "$VOD_STATUS" ]; then
  add_fail "VOD start request produced no HTTP status (network error)"
else
  note "POST /v1/stream/vod/start -> HTTP $VOD_STATUS${VOD_CODE:+ ($VOD_CODE)}"
  if [ "$VOD_STATUS" = "400" ] && [ "$VOD_CODE" = "invalid_vod_id" ]; then
    VOD_FAIL_REGRESSION=1
    add_fail "HTTP 400 invalid_vod_id for a well-formed id '$VOD_ID' - client bundle regression (frontend posting wrong field)"
  else
    ok "VOD start accepted the id format (no 400 invalid_vod_id)"
  fi
fi

# --- Check C: deploy-mismatch classification (Req 33.3) ---------------------
if [ "$LIVE_PASS" = "1" ] && [ "$VOD_FAIL_REGRESSION" = "1" ]; then
  echo ""
  echo "smoke-release-bundle: ===================================================" >&2
  echo "smoke-release-bundle: DEPLOY MISMATCH - live path serves but VOD start" >&2
  echo "smoke-release-bundle: returned 400 invalid_vod_id for a valid id." >&2
  echo "smoke-release-bundle: Treat as a STALE FRONTEND BUNDLE, not a Twitch" >&2
  echo "smoke-release-bundle: content issue. Rebuild/redeploy the frontend image" >&2
  echo "smoke-release-bundle: and verify the served entry matches CI (Check A)." >&2
  echo "smoke-release-bundle: ===================================================" >&2
fi

echo ""
if [ "$FAILURES" -gt 0 ]; then
  echo "smoke-release-bundle: FAILED ($FAILURES issue(s))" >&2
  exit 1
fi
echo "smoke-release-bundle: all checks passed"
exit 0
