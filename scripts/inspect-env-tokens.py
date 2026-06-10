#!/usr/bin/env python3
from __future__ import annotations

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
text = (ROOT / ".env").read_text(encoding="utf-8")
keys = [
    "CLIPPER_TWITCH_USER_ACCESS_TOKEN",
    "CLIPPER_TWITCH_CLIENT_ID",
    "TWITCH_OAUTH_CLIENT_ID",
    "TWITCH_USER_ACCESS_TOKEN",
    "TWITCH_DEV_ACCESS_TOKEN",
]
for key in keys:
    m = re.search(rf"^{re.escape(key)}=(.*)$", text, re.M)
    val = m.group(1).strip().strip('"').strip("'") if m else ""
    print(f"{key}: set={bool(val)} len={len(val)} prefix={val[:4] if len(val) >= 4 else ''}")
