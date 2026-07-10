#!/usr/bin/env python3
"""Suggest narrow test commands when the agent stops after editing code."""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

RULES: list[tuple[str, str]] = [
    ("internal/chat/", "go test ./internal/chat/..."),
    ("internal/video/", "go test ./internal/video/..."),
    ("internal/metadata/", "go test ./internal/metadata/..."),
    ("frontend/", "cd frontend && npm run build"),
    ("deploy/", "docker compose --env-file .env -f deploy/docker-compose.yml -f deploy/docker-compose.local-tunnel.yml config"),
    ("scripts/check-product-boundary.sh", "make product-boundary-strict"),
]


def changed_paths() -> list[str]:
    proc = subprocess.run(
        ["git", "diff", "--name-only", "HEAD"],
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        proc = subprocess.run(["git", "status", "--porcelain"], capture_output=True, text=True, check=False)
    paths: list[str] = []
    for line in proc.stdout.splitlines():
        line = line.strip()
        if not line:
            continue
        if " -> " in line:
            line = line.split(" -> ", 1)[1]
        parts = line.split(maxsplit=1)
        path = parts[-1].replace("\\", "/")
        paths.append(path)
    return paths


def main() -> int:
    _ = json.load(sys.stdin) if not sys.stdin.isatty() else {}
    paths = changed_paths()
    if not paths:
        return 0

    commands: list[str] = []
    for prefix, cmd in RULES:
        if any(p.startswith(prefix) for p in paths):
            if cmd not in commands:
                commands.append(cmd)

    if len({p.split("/")[0] for p in paths if "/" in p}) > 2:
        commands.append("go test ./...")

    if not commands:
        return 0

    body = "Suggested narrow verification before finishing:\n" + "\n".join(f"- `{c}`" for c in commands)
    print(json.dumps({"followup_message": body}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
