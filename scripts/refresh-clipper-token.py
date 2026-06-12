#!/usr/bin/env python3
from __future__ import annotations

import json
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


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


def write_env_value(path: Path, key: str, value: str) -> None:
    lines = path.read_text(encoding="utf-8").splitlines()
    prefix = f"{key}="
    replaced = False
    for idx, line in enumerate(lines):
        if line.startswith(prefix):
            lines[idx] = prefix + value
            replaced = True
            break
    if not replaced:
        lines.append(prefix + value)
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def validate_token(token: str) -> dict:
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
                "client_id": body.get("client_id") or "",
                "scopes": body.get("scopes") or [],
                "expires_in": body.get("expires_in"),
            }
    except urllib.error.HTTPError as exc:
        return {
            "ok": False,
            "status": exc.code,
            "body": exc.read().decode("utf-8", errors="replace")[:160],
        }


def refresh_access_token(client_id: str, client_secret: str, refresh_token: str) -> dict:
    payload = urllib.parse.urlencode(
        {
            "grant_type": "refresh_token",
            "refresh_token": refresh_token,
            "client_id": client_id,
            "client_secret": client_secret,
        }
    ).encode("utf-8")
    req = urllib.request.Request(
        "https://id.twitch.tv/oauth2/token",
        data=payload,
        method="POST",
        headers={"Content-Type": "application/x-www-form-urlencoded"},
    )
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            return json.loads(resp.read().decode())
    except urllib.error.HTTPError as exc:
        return {
            "error": "refresh_http_error",
            "status": exc.code,
            "body": exc.read().decode("utf-8", errors="replace")[:200],
        }


def cli_config_path() -> Path:
    appdata = Path("/mnt/c/Users/Aron/AppData/Roaming/twitch-cli/.twitch-cli.env")
    if appdata.exists():
        return appdata
    return Path.home() / ".twitch-cli" / ".twitch-cli.env"


def main() -> int:
    env_path = ROOT / ".env"
    env = read_kv(env_path)
    cli = read_kv(cli_config_path())

    client_id = env.get("TWITCH_OAUTH_CLIENT_ID") or cli.get("CLIENTID") or ""
    client_secret = env.get("TWITCH_OAUTH_CLIENT_SECRET") or cli.get("CLIENTSECRET") or ""
    refresh_token = env.get("CLIPPER_TWITCH_REFRESH_TOKEN") or cli.get("REFRESHTOKEN") or ""
    current = env.get("CLIPPER_TWITCH_USER_ACCESS_TOKEN") or cli.get("ACCESSTOKEN") or ""

    before = validate_token(current)

    if before.get("ok"):
        print(json.dumps({"status": "already_valid", "expires_in": before.get("expires_in")}))
        return 0

    if not client_id or not client_secret or not refresh_token:
        print(
            json.dumps(
                {
                    "status": "refresh_unavailable",
                    "reason": "missing_refresh_inputs",
                    "has_client_id": bool(client_id),
                    "has_client_secret": bool(client_secret),
                    "has_refresh_token": bool(refresh_token),
                    "remediation": "Run make twitch-local-auth and approve the Twitch login.",
                }
            )
        )
        return 1

    refreshed = refresh_access_token(client_id, client_secret, refresh_token)
    access_token = str(refreshed.get("access_token") or "")
    if not access_token:
        print(json.dumps({"status": "refresh_failed", **refreshed}))
        return 1

    write_env_value(env_path, "CLIPPER_TWITCH_USER_ACCESS_TOKEN", access_token)
    write_env_value(env_path, "CLIPPER_TWITCH_CLIENT_ID", client_id)
    if refreshed.get("refresh_token"):
        write_env_value(env_path, "CLIPPER_TWITCH_REFRESH_TOKEN", str(refreshed["refresh_token"]))
        # Keep CLI cache aligned for future refreshes.
        write_env_value(cli_config_path(), "ACCESSTOKEN", access_token)
        write_env_value(cli_config_path(), "REFRESHTOKEN", str(refreshed["refresh_token"]))

    after = validate_token(access_token)

    result = {
        "status": "refreshed" if after.get("ok") else "refreshed_but_invalid",
        "token_prefix": access_token[:4] if len(access_token) >= 4 else "",
        "token_len": len(access_token),
        "validate": after,
    }
    print(json.dumps(result))
    return 0 if after.get("ok") else 1


if __name__ == "__main__":
    raise SystemExit(main())
