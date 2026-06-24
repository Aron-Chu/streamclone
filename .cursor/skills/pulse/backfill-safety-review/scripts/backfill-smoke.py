#!/usr/bin/env python3
"""Light smoke: extension health + optional pulse payload shape (no secrets)."""

from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.request

DEFAULT_BASE = "http://localhost:8090"


def get_json(url: str) -> dict:
    req = urllib.request.Request(url, headers={"Accept": "application/json"})
    with urllib.request.urlopen(req, timeout=8) as resp:
        return json.loads(resp.read().decode())


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--base", default=DEFAULT_BASE)
    parser.add_argument("--login", default="")
    args = parser.parse_args()
    base = args.base.rstrip("/")

    health = get_json(f"{base}/v1/extension/health")
    print("health:", json.dumps(health, indent=2)[:800])

    if args.login:
        try:
            pulse = get_json(f"{base}/v1/extension/pulse/channels/{args.login}?window=full")
            keys = {
                k: pulse.get(k)
                for k in (
                    "coverage",
                    "coverageStartOffsetSeconds",
                    "vodId",
                    "tracking",
                    "isLive",
                    "streamId",
                )
                if k in pulse
            }
            keys["peaks_count"] = len(pulse.get("peaks") or [])
            print("pulse subset:", json.dumps(keys, indent=2))
        except urllib.error.HTTPError as exc:
            print(f"pulse request failed: {exc.code}", file=sys.stderr)
            return 1

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
