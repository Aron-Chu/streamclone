#!/usr/bin/env python3
"""Remind to update sibling tasks.md only after acceptance criteria pass."""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

TASKS = Path("../streamclone-pulse/docs/website-portal/tasks.md")


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
        paths.append(line.split(maxsplit=1)[-1].replace("\\", "/"))
    return paths


def main() -> int:
    _ = json.load(sys.stdin) if not sys.stdin.isatty() else {}
    paths = changed_paths()
    pulse_touched = any(
        p.startswith(prefix)
        for p in paths
        for prefix in ("internal/analytics/", "packages/pulse-core/", "deploy/Caddyfile", "deploy/docker-compose.bearhost")
    )
    if not pulse_touched:
        return 0

    tasks_path = "streamclone-pulse/docs/website-portal/tasks.md"
    body = (
        f"If this work closes a portal/infra TASK-ID, update `{tasks_path}` checkbox only after "
        "acceptance criteria and tests pass. Backend-only changes may not need tasks.md updates."
    )
    print(json.dumps({"followup_message": body}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
