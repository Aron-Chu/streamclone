#!/usr/bin/env python3
"""Validate clipper Twitch token without printing secrets."""
from __future__ import annotations

import json
import sys
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "clipper"))

from liveclipper.config import load_config  # noqa: E402


def main() -> int:
    cfg = load_config()
    cid = cfg.twitch_client_id
    tok = cfg.twitch_user_access_token
    out: dict = {
        "client_id_set": bool(cid),
        "client_id_len": len(cid),
        "client_id_prefix": cid[:4] if len(cid) >= 4 else "",
        "token_set": bool(tok),
        "token_len": len(tok),
        "token_has_whitespace": tok != tok.strip() if tok else False,
    }
    if not tok:
        out["validate_error"] = "token_missing"
        print(json.dumps(out))
        return 1
    req = urllib.request.Request(
        "https://id.twitch.tv/oauth2/validate",
        headers={"Authorization": "Bearer " + tok.strip()},
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            body = json.loads(resp.read().decode())
            out.update(
                {
                    "validate_status": resp.status,
                    "validate_client_id_prefix": (body.get("client_id") or "")[:4],
                    "client_id_match": (body.get("client_id") or "") == cid,
                    "validate_scopes": body.get("scopes") or [],
                    "expires_in": body.get("expires_in"),
                    "has_clips_edit": "clips:edit" in (body.get("scopes") or []),
                }
            )
    except urllib.error.HTTPError as exc:
        out.update(
            {
                "validate_http_error": exc.code,
                "validate_body": exc.read().decode("utf-8", errors="replace")[:200],
            }
        )
        print(json.dumps(out))
        return 1
    print(json.dumps(out))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
