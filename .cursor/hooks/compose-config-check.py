#!/usr/bin/env python3
"""Validate docker compose config after deploy/ file edits."""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except json.JSONDecodeError:
        return 0

    file_path = str(payload.get("file_path") or payload.get("path") or "").replace("\\", "/")
    if not file_path.startswith("deploy/"):
        return 0

    repo = Path.cwd()
    env_file = repo / ".env"
    if not env_file.exists():
        env_file = repo / ".env.example"

    cmd = [
        "docker",
        "compose",
        "--env-file",
        str(env_file),
        "-f",
        str(repo / "deploy/docker-compose.yml"),
        "-f",
        str(repo / "deploy/docker-compose.local-tunnel.yml"),
        "config",
    ]
    proc = subprocess.run(cmd, cwd=repo, capture_output=True, text=True, check=False)
    if proc.returncode == 0:
        print(json.dumps({"additional_context": "docker compose config validated successfully after deploy/ edit."}))
        return 0

    print(
        json.dumps(
            {
                "additional_context": (
                    "docker compose config failed after deploy/ edit. Fix compose/Caddy errors before continuing.\n"
                    + proc.stderr[-2000:]
                )
            }
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
