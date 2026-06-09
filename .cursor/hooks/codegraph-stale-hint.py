#!/usr/bin/env python3
"""Suggest codegraph rebuild when indexed Go/TS sources were edited."""

from __future__ import annotations

import json
import sys
from pathlib import Path

WATCH_SUFFIXES = {".go", ".ts", ".tsx"}
WATCH_PREFIXES = ("internal/", "cmd/", "frontend/src/")
DB_REL = Path(".codegraph/streamclone.kuzu")


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except json.JSONDecodeError:
        return 0

    file_path = str(payload.get("file_path") or payload.get("path") or "").replace("\\", "/")
    if not file_path:
        return 0

    normalized = file_path.lstrip("./")
    if not any(normalized.startswith(prefix) for prefix in WATCH_PREFIXES):
        return 0
    if Path(normalized).suffix not in WATCH_SUFFIXES:
        return 0

    repo = Path.cwd()
    db = repo / DB_REL
    edited = repo / normalized
    if not edited.exists():
        return 0

    stale = not db.exists() or db.stat().st_mtime < edited.stat().st_mtime
    if not stale:
        return 0

    print(
        json.dumps(
            {
                "additional_context": (
                    f"Edited `{normalized}` may be ahead of the code graph index. "
                    "Run MCP `rebuild_graph` on streamclone-codegraph or `make codegraph` before cross-package symbol lookup."
                )
            }
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
