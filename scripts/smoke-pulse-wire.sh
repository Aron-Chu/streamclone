#!/usr/bin/env bash
# Pulse Wire API smoke — run with stack up at http://localhost:8090 and PULSE_WIRE_ENABLED=true.
set -euo pipefail

BASE="${PULSE_WIRE_SMOKE_BASE:-http://localhost:8090}"

fail() {
  echo "smoke-pulse-wire: $1" >&2
  exit 1
}

curl_json() {
  local url="$1"
  curl --connect-timeout 3 --max-time 15 -fsS "$url"
}

echo "Pulse Wire smoke against $BASE"

health="$(curl_json "$BASE/v1/pulse-wire/source-health" || true)"
if [[ -z "$health" ]]; then
  fail "source-health unreachable — is storygraph up and PULSE_WIRE_ENABLED=true?"
fi
echo "  source-health ok"

if ! echo "$health" | grep -q 'windowScoreCompute'; then
  fail "source-health missing windowScoreCompute block"
fi
echo "  ok windowScoreCompute exposed"

for path in \
  "/v1/pulse-wire/feed?window=24h&sort=rank" \
  "/v1/pulse-wire/feed?window=today&sort=rank" \
  "/v1/pulse-wire/feed?window=7d&sort=rank" \
  "/v1/pulse-wire/trending-streamers?window=7d" \
  "/v1/pulse-wire/rising-streamers?window=today&limit=5" \
  "/v1/pulse-wire/daily" \
  "/v1/pulse-wire/edition?window=24h" \
  "/v1/pulse-wire/edition?window=today" \
  "/v1/pulse-wire/edition?window=7d" \
  "/v1/pulse-wire/source-mix?window=24h" \
  "/v1/pulse-wire/community?window=24h&sort=hot&limit=5" \
  "/v1/pulse-wire/clips/top?window=24h&limit=5" \
  "/v1/pulse-wire/bans?window=24h&limit=5" \
  "/v1/pulse-wire/evidence/unlinked?window=24h&limit=5" \
  "/v1/pulse-wire/rising?window=24h"
do
  body="$(curl_json "$BASE$path")"
  if [[ -z "$body" ]]; then
    fail "empty response for $path"
  fi
  echo "  ok $path"
done

community="$(curl_json "$BASE/v1/pulse-wire/community?window=24h&sort=hot&limit=5")"
if echo "$community" | grep -Eq 'clips-media-assets2\.twitch\.tv/1u[a-z0-9]+-preview'; then
  fail "community items contain fake twitch preview derived from reddit post id"
fi
echo "  ok community items avoid fake reddit-id twitch previews"

clips="$(curl_json "$BASE/v1/pulse-wire/clips/top?window=24h&limit=5")"
if echo "$clips" | grep -q '"displayThumbnailUrl"'; then
  echo "  ok clips/top exposes displayThumbnailUrl"
else
  echo "  note clips/top has no displayThumbnailUrl (empty window is ok)"
fi

edition="$(curl_json "$BASE/v1/pulse-wire/edition?window=24h")"
if ! echo "$edition" | grep -q '"sections"'; then
  fail "edition missing sections array"
fi
if ! echo "$edition" | grep -q '"sampleStatus"'; then
  fail "edition missing sampleStatus"
fi
echo "  ok edition sections + sampleStatus"

code="$(curl -o /dev/null -s -w '%{http_code}' "$BASE/v1/pulse-wire/feed?window=30d")"
if [[ "$code" != "400" ]]; then
  fail "expected 400 for window=30d, got $code"
fi
echo "  ok invalid window returns 400"

echo "smoke-pulse-wire: passed"
