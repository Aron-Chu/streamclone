#!/usr/bin/env python3
"""Run gofmt -w on a single edited Go file."""

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
    if not file_path.endswith(".go"):
        return 0

    target = Path.cwd() / file_path
    if not target.is_file():
        return 0

    proc = subprocess.run(["gofmt", "-w", str(target)], capture_output=True, text=True, check=False)
    if proc.returncode != 0:
        print(json.dumps({"additional_context": f"gofmt failed on {file_path}: {proc.stderr[:500]}"}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
