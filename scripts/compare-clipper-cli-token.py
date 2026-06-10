#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
import urllib.error
import urllib.request
from pathlib import Path


def read_kv(path: Path) -> dict[str, str]:
    out: dict[str, str] = {}
    if not path.exists():
        return out
    for line in path.read_text(encoding="utf-8").splitlines():
        if "=" not in line:
            continue
        key, value = line.split("=", 1)
        out[key.strip()] = value.strip().strip('"').strip("'")
    return out


def validate(token: str) -> dict:
    if not token:
        return {"ok": False, "reason": "missing"}
    req = urllib.request.Request(
        "https://id.twitch.tv/oauth2/validate",
        headers={"Authorization": "Bearer " + token.strip()},
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            body = json.loads(resp.read().decode())
            return {
                "ok": True,
                "status": resp.status,
                "client_id_prefix": (body.get("client_id") or "")[:4],
                "scopes": body.get("scopes") or [],
                "expires_in": body.get("expires_in"),
            }
    except urllib.error.HTTPError as exc:
        return {
            "ok": False,
            "status": exc.code,
            "body": exc.read().decode("utf-8", errors="replace")[:160],
        }


def main() -> int:
    root = Path(__file__).resolve().parents[1]
    env = read_kv(root / ".env")
    cli = read_kv(Path("/mnt/c/Users/Aron/AppData/Roaming/twitch-cli/.twitch-cli.env"))
    if not cli:
        cli = read_kv(Path.home() / ".twitch-cli" / ".twitch-cli.env")

    clipper_token = env.get("CLIPPER_TWITCH_USER_ACCESS_TOKEN", "")
    cli_token = cli.get("ACCESSTOKEN", "")
    oauth_client = env.get("TWITCH_OAUTH_CLIENT_ID", "")
    cli_client = cli.get("CLIENTID", "")

    report = {
        "clipper_token_prefix": clipper_token[:4],
        "clipper_token_len": len(clipper_token),
        "cli_token_prefix": cli_token[:4],
        "cli_token_len": len(cli_token),
        "tokens_match": clipper_token == cli_token,
        "oauth_client_prefix": oauth_client[:4],
        "cli_client_prefix": cli_client[:4],
        "client_ids_match": oauth_client == cli_client,
        "clipper_validate": validate(clipper_token),
        "cli_validate": validate(cli_token),
    }
    print(json.dumps(report, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
