#!/usr/bin/env python3
import json
import re
import sys
import urllib.parse
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
clip_id = sys.argv[1] if len(sys.argv) > 1 else "HealthyMuddyWallabyAsianGlow-x8GN9gMEBSv_Vgsp"
text = (ROOT / ".env").read_text(encoding="utf-8")
env = {m.group(1): m.group(2).strip().strip('"').strip("'") for m in re.finditer(r"^([A-Z0-9_]+)=(.*)$", text, re.M)}
cid = env.get("CLIPPER_TWITCH_CLIENT_ID") or env.get("TWITCH_OAUTH_CLIENT_ID", "")
tok = env.get("CLIPPER_TWITCH_USER_ACCESS_TOKEN", "")
url = "https://api.twitch.tv/helix/clips?" + urllib.parse.urlencode({"id": clip_id})
req = urllib.request.Request(url, headers={"Client-Id": cid, "Authorization": "Bearer " + tok, "Accept": "application/json"})
with urllib.request.urlopen(req, timeout=15) as r:
    body = json.loads(r.read().decode())
item = (body.get("data") or [{}])[0]
print(json.dumps({"clip_id": clip_id, "has_url": bool(item.get("url")), "url": item.get("url"), "duration": item.get("duration"), "title": item.get("title")}, indent=2))
