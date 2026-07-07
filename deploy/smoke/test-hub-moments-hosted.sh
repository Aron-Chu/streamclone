#!/usr/bin/env bash
# Hosted smoke: /v1/public/hub/moments returns sanitized bucket rows with viewers/emotes.
set -euo pipefail

API_BASE="${API_BASE:-https://api.streampulse.stream}"
NOW_MS="$(python3 - <<'PY'
import time
print(int(time.time() * 1000))
PY
)"
BUCKET_MS=$((6 * 60 * 1000))
BUCKET_T=$(( (NOW_MS / BUCKET_MS) * BUCKET_MS - BUCKET_MS ))

URL="${API_BASE}/v1/public/hub/moments?bucketT=${BUCKET_T}"
echo "GET ${URL}"

BODY="$(curl -fsS "${URL}")"
python3 - <<'PY' "${BODY}"
import json, sys
body = json.loads(sys.argv[1])
assert isinstance(body, dict), "response must be object"
status = body.get("status")
assert status in {"ready", "empty", "no_data", "degraded"}, f"unexpected status: {status}"
moments = body.get("moments") or []
print(f"status={status} moments={len(moments)}")
if moments:
    row = moments[0]
    for key in ("login", "score", "label"):
        assert key in row, f"missing {key} on moment row"
    if "viewers" in row:
        assert isinstance(row["viewers"], (int, float)), "viewers must be numeric when present"
    if "topEmotes" in row and row["topEmotes"]:
        emote = row["topEmotes"][0]
        assert "name" in emote and "count" in emote, "topEmotes entries need name/count"
print("hub moments smoke ok")
PY

echo "OK: hosted hub moments shape"
